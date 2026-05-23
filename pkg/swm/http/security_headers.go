package http

import (
	"github.com/gin-gonic/gin"
)

// SecurityHeaders 是统一的安全响应头中间件。
//
// CSP 仅允许同源资源加载，配合现代化前端（无外部 CDN）。
func SecurityHeaders() gin.HandlerFunc {
	csp := "default-src 'self'; " +
		"script-src 'self'; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; " +
		"font-src 'self' data:; " +
		"connect-src 'self'; " +
		"frame-ancestors 'none'; " +
		"base-uri 'self'; " +
		"form-action 'self'"

	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=(), payment=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		c.Next()
	}
}
