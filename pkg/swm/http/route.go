package http

import (
	"embed"
	"encoding/gob"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"tacacs/pkg/public/cfg"
	"tacacs/pkg/public/log"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func route(app *gin.Engine, staticFS embed.FS) {
	gob.Register(time.Time{})

	// 1. 安全 headers 最先生效
	app.Use(SecurityHeaders())

	// 2. Session：随机 key + Secure/HttpOnly/SameSite Strict
	store := cookie.NewStore(loadOrCreateSessionKey())
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   int((time.Duration(cfg.SwmConfig().SessionTimeOut) * time.Minute).Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	app.Use(sessions.Sessions(sessionName, store))

	// 3. 模板与静态资源（来自编译期嵌入的 staticFS，不再依赖磁盘 ./static/）
	tmpl, err := template.ParseFS(staticFS, "static/*.html")
	if err != nil {
		log.Logger.Errorf("parse embedded html templates fail: %v", err)
		return
	}
	app.SetHTMLTemplate(tmpl)

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Logger.Errorf("sub embedded static fs fail: %v", err)
		return
	}
	app.StaticFS("/static", http.FS(staticSub))

	app.Use(func(c *gin.Context) {
		p := c.Request.URL.Path
		// 静态资源用 no-cache + 必须重新校验：浏览器仍可缓存，但每次都回源带
		// If-Modified-Since 询问，未变 → 304 用本地副本；变了 → 200 取新版。
		// 这样既不浪费带宽又能让前端发版后立刻生效，避免出现 HTML/JS 不同步。
		if strings.HasPrefix(p, "/static/") {
			c.Header("Cache-Control", "no-cache, must-revalidate")
		}
		c.Next()
	})

	// 4. 公开路由（登录前可达），登录与注册接口加限流
	app.GET("/login", showLoginPage)
	app.POST("/login", LoginRateLimit(), handleLogin)
	app.GET("/logout", handleLogout)
	app.POST("/create-user", LoginRateLimit(), handleCreateUser)
	app.GET("/check-session", checkSession)

	// 5. 受保护路由：登录态 + CSRF
	authed := app.Group("/")
	authed.Use(AuthRequired())
	authed.Use(CSRFProtect())
	authed.GET("/", Index)

	// 6. /tacacs/* 反向代理：登录态 + 路径 ACL + 身份注入
	proxy, err := newTacacsProxy()
	if err != nil {
		log.Logger.Errorf("init tacacs proxy fail: %v", err)
		return
	}
	authed.Any("/tacacs/*proxyPath", TacacsProxyHandler(proxy))
}
