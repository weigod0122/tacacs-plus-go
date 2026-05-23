package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"tacacs/pkg/public/cfg"
	"tacacs/pkg/public/log"
	"time"

	"github.com/gin-gonic/gin"
)

// 豁免签名校验的路径（仅健康检查）。纯内网部署，飞书不发回调，无需其他豁免。
var swmAuthExemptPaths = map[string]struct{}{
	"/health": {},
}

// nonce 缓存：值是过期时间戳。后台 goroutine 周期清理，防止长跑 OOM。
var (
	nonceStore     sync.Map
	nonceCleanOnce sync.Once
)

const nonceTTL = 600 * time.Second

func startNonceCleaner() {
	nonceCleanOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(60 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				now := time.Now().Unix()
				nonceStore.Range(func(k, v any) bool {
					if exp, ok := v.(int64); ok && exp <= now {
						nonceStore.Delete(k)
					}
					return true
				})
			}
		}()
	})
}

func swmAuthMiddleware() gin.HandlerFunc {
	startNonceCleaner()

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		if _, ok := swmAuthExemptPaths[path]; ok {
			c.Next()
			return
		}

		conf := cfg.ServerConfig().SwmAuth
		secret := conf.ResolveSecret()

		// 算签名结果，enforce=false 时只记日志不拦截，方便灰度
		verifyErr := verifySwmSignature(c.Request, secret, conf.MaxSkewSeconds)

		user := c.Request.Header.Get("X-SwM-User")
		isAdminHeader := c.Request.Header.Get("X-SwM-Is-Admin")
		isAdmin := isAdminHeader == "1"

		if verifyErr != nil {
			if conf.Enforce {
				AuditLog("swm-auth-fail enforce=true path=%s method=%s ip=%s reason=%v",
					path, c.Request.Method, c.ClientIP(), verifyErr)
				log.Logger.Errorf("swm auth reject: path=%s ip=%s err=%v", path, c.ClientIP(), verifyErr)
				if strings.Contains(verifyErr.Error(), "missing") {
					c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "missing signature"})
				} else {
					c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "invalid signature"})
				}
				c.Abort()
				return
			}
			AuditLog("swm-auth-fail enforce=false path=%s method=%s ip=%s reason=%v",
				path, c.Request.Method, c.ClientIP(), verifyErr)
			log.Logger.Errorf("swm auth would-reject (enforce=false): path=%s ip=%s err=%v", path, c.ClientIP(), verifyErr)
		}

		// enforce=false 时签名失败也继续走 ACL（仍按 header 信任），方便灰度阶段观察
		if user == "" {
			if conf.Enforce {
				c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "missing identity"})
				c.Abort()
				return
			}
		}

		if pathRequiresAdmin(path) && !isAdmin {
			AuditLog("forbidden user=%s path=%s method=%s reason=admin-only", user, path, c.Request.Method)
			if conf.Enforce {
				c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "forbidden"})
				c.Abort()
				return
			}
		}

		if hasBodyMethod(c.Request.Method) {
			body, err := readAndRestoreBody(c.Request)
			if err == nil && len(body) > 0 {
				if path == approvalUpdatePath {
					var p struct {
						Status int `json:"status"`
					}
					if json.Unmarshal(body, &p) == nil && p.Status != approvalStatusClose && !isAdmin {
						AuditLog("forbidden user=%s path=%s reason=approval-non-admin status=%d", user, path, p.Status)
						if conf.Enforce {
							c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "forbidden"})
							c.Abort()
							return
						}
					}
				}
				if _, ok := ownershipBodyUserPaths[path]; ok && !isAdmin {
					var p struct {
						User string `json:"user"`
					}
					if json.Unmarshal(body, &p) == nil && p.User != "" && p.User != user {
						AuditLog("forbidden operator=%s path=%s reason=cross-user target=%s", user, path, p.User)
						if conf.Enforce {
							c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "forbidden"})
							c.Abort()
							return
						}
					}
				}
			}
		}

		if isAdmin && hasBodyMethod(c.Request.Method) {
			AuditLog("admin-action user=%s path=%s method=%s", user, path, c.Request.Method)
		}

		c.Next()
	}
}

// verifySwmSignature 校验 X-SwM-Signature 头。返回 nil 表示通过。
// 签名格式：X-SwM-Signature: t=<unix_sec>,n=<nonce>,v1=<base64-sig>
// canonical: method\npath\nts\nnonce\nuser\nisAdmin\nsha256_hex(body)
func verifySwmSignature(r *http.Request, secret string, maxSkewSeconds int) error {
	if secret == "" {
		return errMissingSecret
	}
	sigHeader := r.Header.Get("X-SwM-Signature")
	if sigHeader == "" {
		return errMissingSignature
	}
	ts, nonce, sig, err := parseSwmSigHeader(sigHeader)
	if err != nil {
		return err
	}

	if maxSkewSeconds <= 0 {
		maxSkewSeconds = 300
	}
	tsInt, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errBadTimestamp
	}
	now := time.Now().Unix()
	if abs64(now-tsInt) > int64(maxSkewSeconds) {
		return errStaleTimestamp
	}

	if _, dup := nonceStore.LoadOrStore(nonce, now+int64(nonceTTL/time.Second)); dup {
		return errReplayedNonce
	}

	body, err := readAndRestoreBody(r)
	if err != nil {
		body = nil
	}
	bodyHash := sha256.Sum256(body)

	canonical := strings.Join([]string{
		r.Method,
		r.URL.Path,
		ts,
		nonce,
		r.Header.Get("X-SwM-User"),
		r.Header.Get("X-SwM-Is-Admin"),
		hex.EncodeToString(bodyHash[:]),
	}, "\n")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return errBadSignature
	}
	return nil
}

func parseSwmSigHeader(h string) (ts, nonce, sig string, err error) {
	parts := strings.Split(h, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch {
		case strings.HasPrefix(p, "t="):
			ts = p[2:]
		case strings.HasPrefix(p, "n="):
			nonce = p[2:]
		case strings.HasPrefix(p, "v1="):
			sig = p[3:]
		}
	}
	if ts == "" || nonce == "" || sig == "" {
		return "", "", "", errBadHeaderFormat
	}
	return ts, nonce, sig, nil
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// 各种错误用 sentinel，方便上层判断是否是 "missing" 类
var (
	errMissingSecret    = &swmAuthErr{msg: "shared secret not configured"}
	errMissingSignature = &swmAuthErr{msg: "missing X-SwM-Signature header"}
	errBadHeaderFormat  = &swmAuthErr{msg: "bad signature header format"}
	errBadTimestamp     = &swmAuthErr{msg: "bad timestamp"}
	errStaleTimestamp   = &swmAuthErr{msg: "stale timestamp"}
	errReplayedNonce    = &swmAuthErr{msg: "replayed nonce"}
	errBadSignature     = &swmAuthErr{msg: "signature mismatch"}
)

type swmAuthErr struct{ msg string }

func (e *swmAuthErr) Error() string { return e.msg }
