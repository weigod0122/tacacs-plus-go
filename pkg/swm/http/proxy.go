package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"tacacs/pkg/public/cfg"
	"tacacs/pkg/public/log"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// adminOnlyPrefixes 是只有管理员可访问的代理路径前缀。
var adminOnlyPrefixes = []string{
	"/tacacs/user/get-admin",
	"/tacacs/user/delete",
	"/tacacs/user/create",
	"/tacacs/user/clear/",
	"/tacacs/log/",
}

// adminOnlyExact 是仅管理员可调用的精确路径。
// 注：模板（role/server/command）的写操作沿用原前端"任何登录用户均可操作"的语义，
// 不在此列表；后端 tacacs_manager 自身做最终权限校验。
var adminOnlyExact = map[string]struct{}{
	"/tacacs/user/reset/password": {},
	"/tacacs/meta/refresh":        {},
}

// ownershipBodyUserPaths 是 body 里有 "user" 字段、非管理员调用时该字段必须 ==
// 操作者本人的写接口。防止 A 用 body.user=B 越权改 B 的密码 / 备注 / 基础信息，
// 或以 B 的名义提交审批。
var ownershipBodyUserPaths = map[string]struct{}{
	"/tacacs/user/update/password":  {},
	"/tacacs/user/update/notes":     {},
	"/tacacs/user/update/basicInfo": {},
	"/tacacs/approval/create":       {},
}

const (
	approvalUpdatePath  = "/tacacs/approval/update"
	approvalStatusClose = 0 // 仅 close 自己工单时允许非管理员调 update
	maxBodyPeek         = 256 * 1024
)

const adminCacheTTL = 30 * time.Second

var (
	adminCacheLock sync.RWMutex
	adminCache     []string
	adminCacheAt   time.Time
)

// getAdminUsers 拉管理员列表，30s TTL 内存缓存。后端不可达时返回上次缓存。
func getAdminUsers() []string {
	adminCacheLock.RLock()
	if adminCache != nil && time.Since(adminCacheAt) < adminCacheTTL {
		defer adminCacheLock.RUnlock()
		return adminCache
	}
	adminCacheLock.RUnlock()

	adminCacheLock.Lock()
	defer adminCacheLock.Unlock()
	if adminCache != nil && time.Since(adminCacheAt) < adminCacheTTL {
		return adminCache
	}

	endpoint := fmt.Sprintf("%v/tacacs/user/get-admin", cfg.SwmConfig().TacacsManagerUrl)
	msg, err := signedInternalGet(endpoint)
	if err != nil {
		log.Logger.Errorf("refresh admin cache fail: %v", err)
		return adminCache
	}
	var admins []string
	if err := json.Unmarshal(msg, &admins); err != nil {
		log.Logger.Errorf("unmarshal admin list fail: %v", err)
		return adminCache
	}
	adminCache = admins
	adminCacheAt = time.Now()
	return adminCache
}

func pathRequiresAdmin(path string) bool {
	if _, ok := adminOnlyExact[path]; ok {
		return true
	}
	for _, p := range adminOnlyPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// newTacacsProxy 构建反代到 tacacs_manager 的 ReverseProxy。
// 注意：剥除入站伪造 header 与注入可信身份 header 都在 handler 里完成（在
// proxy.ServeHTTP 调用之前），所以这里的 Director 用默认实现即可——避免之前
// "Director 把 handler 注入的 X-SwM-* 一并删掉" 的 bug。
func newTacacsProxy() (*httputil.ReverseProxy, error) {
	target, err := url.Parse(cfg.SwmConfig().TacacsManagerUrl)
	if err != nil {
		return nil, err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Logger.Errorf("proxy error: %s %s -> %v", r.Method, r.URL.Path, err)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"code":502,"msg":"后端服务不可达"}`))
	}

	return proxy, nil
}

// TacacsProxyHandler 是 /tacacs/* 的处理器：
//  1. 会话校验
//  2. 路径级 ACL（admin-only 前缀/精确路径）
//  3. body 级 ACL（approval/update 状态校验、self-only 写接口归属校验）
//  4. 剥除入站伪造的 X-SwM-* header（防客户端冒充）
//  5. 注入可信的 X-SwM-User / X-SwM-Is-Admin 给后端校验
func TacacsProxyHandler(proxy *httputil.ReverseProxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		username, _ := session.Get("username").(string)
		if username == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "未登录"})
			c.Abort()
			return
		}

		admins := getAdminUsers()
		isAdmin := false
		for _, a := range admins {
			if a == username {
				isAdmin = true
				break
			}
		}

		// (2) 路径级 ACL
		if pathRequiresAdmin(c.Request.URL.Path) && !isAdmin {
			log.Logger.Errorf("forbidden: user=%s path=%s", username, c.Request.URL.Path)
			AuditLog("forbidden user=%s path=%s method=%s", username, c.Request.URL.Path, c.Request.Method)
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "无权访问"})
			c.Abort()
			return
		}

		// (3) body 级 ACL —— 只对带 body 的方法做
		if hasBodyMethod(c.Request.Method) {
			body, err := readAndRestoreBody(c.Request)
			if err == nil && len(body) > 0 {
				// /tacacs/approval/update：只有 status=0（关闭工单）允许非管理员调
				if c.Request.URL.Path == approvalUpdatePath {
					var p struct {
						Status int `json:"status"`
					}
					if json.Unmarshal(body, &p) == nil && p.Status != approvalStatusClose && !isAdmin {
						log.Logger.Errorf("forbidden approval-update: user=%s status=%d", username, p.Status)
						AuditLog("forbidden user=%s path=%s status=%d", username, c.Request.URL.Path, p.Status)
						c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "无权审批工单"})
						c.Abort()
						return
					}
				}

				// 归属检查：body.user 必须 == 操作者（admin 豁免）
				if _, ok := ownershipBodyUserPaths[c.Request.URL.Path]; ok && !isAdmin {
					var p struct {
						User string `json:"user"`
					}
					if json.Unmarshal(body, &p) == nil && p.User != "" && p.User != username {
						log.Logger.Errorf("forbidden cross-user: operator=%s body.user=%s path=%s",
							username, p.User, c.Request.URL.Path)
						AuditLog("forbidden user=%s path=%s reason=cross-user target=%s",
							username, c.Request.URL.Path, p.User)
						c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "无权操作他人数据"})
						c.Abort()
						return
					}
				}
			}
		}

		// (4) 剥除任何入站 X-SwM-* header（防伪造）
		for k := range c.Request.Header {
			if strings.HasPrefix(strings.ToLower(k), "x-swm-") {
				c.Request.Header.Del(k)
			}
		}

		// (5) 注入可信身份。注意：这两步必须在 ServeHTTP 之前做，因为 ServeHTTP
		//    内部会 Clone request，clone 后 Director 已无法再加这些 header。
		c.Request.Header.Set("X-SwM-User", username)
		isAdminVal := "0"
		if isAdmin {
			isAdminVal = "1"
		}
		c.Request.Header.Set("X-SwM-Is-Admin", isAdminVal)

		// (6) 用与 tacacs-server 共享的密钥对身份+body 做 HMAC 签名，防止有人绕过
		//     SwM 直接用伪造 header 调后端。canonical 必须包含 user/isAdmin，否则
		//     攻击者改身份头不影响签名。
		if secret := cfg.SwmConfig().ResolveSwmSecret(); secret != "" {
			signRequest(c.Request, secret, username, isAdminVal)
		}

		// 管理员写操作记审计
		if isAdmin && hasBodyMethod(c.Request.Method) {
			AuditLog("admin-action user=%s path=%s method=%s", username, c.Request.URL.Path, c.Request.Method)
		}

		proxy.ServeHTTP(c.Writer, c.Request)
	}
}

func hasBodyMethod(m string) bool {
	return m == http.MethodPost || m == http.MethodPut || m == http.MethodDelete || m == http.MethodPatch
}

// readAndRestoreBody 读完 body 并把它还原成可重新读取的 ReadCloser，
// 这样后续 proxy 转发时仍能拿到完整 body。最大读 256KB，超出当作"无法解析"
// 跳过 body 级检查（让后端兜底），避免内存炸。
func readAndRestoreBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	limited := io.LimitReader(r.Body, maxBodyPeek+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	_ = r.Body.Close()
	if int64(len(body)) > maxBodyPeek {
		// 太大就不解析了，原样转给后端，且把 body 还回去
		r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), r.Body))
		return nil, fmt.Errorf("body too large for peek")
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	return body, nil
}

// signRequest 给反代请求加 X-SwM-Signature 头，身份用真实登录用户。
// 算法核心走 computeSignatureHeader 共用，与后端 verifySwmSignature 严格一致。
func signRequest(r *http.Request, secret, user, isAdmin string) {
	body, err := readAndRestoreBody(r)
	if err != nil {
		body = nil
	}
	r.Header.Set("X-SwM-Signature", computeSignatureHeader(r.Method, r.URL.Path, user, isAdmin, body, secret))
}
