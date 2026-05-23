package http

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	csrfCookieName = "csrf_token"
	csrfHeaderName = "X-CSRF-Token"
	csrfSessionKey = "csrf"
)

// EnsureCSRFToken 确保当前会话有 CSRF token，并写入非 HttpOnly cookie 让前端 JS 读取。
// 登录成功后调用一次即可，后续每个写请求会校验 cookie 与 header 是否匹配。
func EnsureCSRFToken(c *gin.Context) string {
	session := sessions.Default(c)
	if v, ok := session.Get(csrfSessionKey).(string); ok && v != "" {
		setCSRFCookie(c, v)
		return v
	}
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	token := base64.RawURLEncoding.EncodeToString(buf)
	session.Set(csrfSessionKey, token)
	_ = session.Save()
	setCSRFCookie(c, token)
	return token
}

func setCSRFCookie(c *gin.Context, token string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		Secure:   true,
		HttpOnly: false,
		SameSite: http.SameSiteStrictMode,
	})
}

// CSRFProtect 实现 double-submit cookie 校验：
//   - 安全方法 (GET/HEAD/OPTIONS) 直接放行；
//   - 其他方法必须携带 X-CSRF-Token header，且值与 session 中的 token 一致。
func CSRFProtect() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
		}

		session := sessions.Default(c)
		expected, _ := session.Get(csrfSessionKey).(string)
		got := c.GetHeader(csrfHeaderName)
		if expected == "" || got == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(got)) != 1 {
			AuditLog("csrf-fail path=%s ip=%s", c.Request.URL.Path, c.ClientIP())
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "CSRF token 校验失败"})
			c.Abort()
			return
		}
		c.Next()
	}
}
