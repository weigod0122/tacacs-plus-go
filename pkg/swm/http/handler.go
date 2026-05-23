package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"tacacs/pkg/public/cfg"
	"tacacs/pkg/public/log"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	sessionName = "user-session"
	sessionKey  = "authenticated"

	maxUsernameLen = 64
	maxPasswordLen = 128
	maxNotesLen    = 500
	maxEmailLen    = 128
	maxPhoneLen    = 32
)

type webInfo struct {
	CurrentUser string
	IsAdmin     bool
}

func Index(c *gin.Context) {
	name, _ := c.Get("username")
	username, _ := name.(string)

	admins := getAdminUsers()
	isAdmin := false
	for _, a := range admins {
		if a == username {
			isAdmin = true
			break
		}
	}

	c.HTML(http.StatusOK, "index.html", webInfo{
		CurrentUser: username,
		IsAdmin:     isAdmin,
	})
}

func showLoginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", gin.H{})
}

func handleLogin(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	password := c.PostForm("password")

	if username == "" || password == "" {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{"error": "用户名或密码不能为空"})
		return
	}
	if len(username) > maxUsernameLen || len(password) > maxPasswordLen {
		c.HTML(http.StatusBadRequest, "login.html", gin.H{"error": "用户名或密码长度超限"})
		return
	}

	endpoint := fmt.Sprintf("%v/tacacs/user/check", cfg.SwmConfig().TacacsManagerUrl)
	body, _ := json.Marshal(map[string]string{
		"user":     username,
		"password": password,
	})
	if _, err := signedInternalPost(endpoint, body); err != nil {
		log.Logger.Errorf("login backend check fail user=%s err=%v", username, err)
		AuditLog("login-fail user=%s ip=%s", username, c.ClientIP())
		RecordLoginFailure(c)
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{"error": "用户名或密码错误"})
		return
	}

	session := sessions.Default(c)
	session.Set(sessionKey, true)
	session.Set("username", username)
	session.Set("last_access", time.Now())
	if err := session.Save(); err != nil {
		log.Logger.Errorf("session save fail user=%s err=%v", username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "无法保存会话"})
		return
	}
	EnsureCSRFToken(c)
	ResetLoginCounter(c)
	AuditLog("login-success user=%s ip=%s", username, c.ClientIP())
	c.Redirect(http.StatusSeeOther, "/")
}

func handleLogout(c *gin.Context) {
	session := sessions.Default(c)
	if name, ok := session.Get("username").(string); ok && name != "" {
		AuditLog("logout user=%s ip=%s", name, c.ClientIP())
	}
	session.Clear()
	_ = session.Save()
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     csrfCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	c.Redirect(http.StatusSeeOther, "/login")
}

func checkSession(c *gin.Context) {
	session := sessions.Default(c)
	authenticated := session.Get(sessionKey)
	lastAccess, ok := session.Get("last_access").(time.Time)
	if authenticated == nil || !ok || time.Since(lastAccess) > time.Duration(cfg.SwmConfig().SessionTimeOut)*time.Minute {
		c.JSON(http.StatusOK, gin.H{"expired": true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"expired": false})
}

// AuthRequired 校验会话存在且未过期；同时刷新 last_access 与 CSRF token cookie。
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		authenticated := session.Get(sessionKey)
		if authenticated == nil {
			redirectOrJSON(c, "/login", "未登录")
			c.Abort()
			return
		}
		lastAccess, ok := session.Get("last_access").(time.Time)
		if !ok || time.Since(lastAccess) > time.Duration(cfg.SwmConfig().SessionTimeOut)*time.Minute {
			session.Clear()
			_ = session.Save()
			redirectOrJSON(c, "/login", "会话已过期")
			c.Abort()
			return
		}
		session.Set("last_access", time.Now())
		_ = session.Save()
		c.Set("username", session.Get("username"))
		EnsureCSRFToken(c)
		c.Next()
	}
}

// redirectOrJSON 对页面请求重定向到登录页，对 XHR/JSON 请求返回 401。
func redirectOrJSON(c *gin.Context, location, msg string) {
	if isAjax(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": msg})
		return
	}
	c.Redirect(http.StatusSeeOther, location)
}

func isAjax(c *gin.Context) bool {
	if c.GetHeader("X-Requested-With") == "XMLHttpRequest" {
		return true
	}
	accept := c.GetHeader("Accept")
	return strings.Contains(accept, "application/json") && !strings.Contains(accept, "text/html")
}

func handleCreateUser(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	password := c.PostForm("password")
	notes := c.PostForm("notes")
	email := strings.TrimSpace(c.PostForm("email"))
	phone := strings.TrimSpace(c.PostForm("phone_number"))

	if username == "" || password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "用户名或密码不能为空"})
		return
	}
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "邮箱不能为空"})
		return
	}
	if phone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "联系电话不能为空"})
		return
	}
	if len(username) > maxUsernameLen || len(password) > maxPasswordLen ||
		len(notes) > maxNotesLen || len(email) > maxEmailLen || len(phone) > maxPhoneLen {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "字段长度超限"})
		return
	}

	endpoint := fmt.Sprintf("%v/tacacs/user/create", cfg.SwmConfig().TacacsManagerUrl)
	body, _ := json.Marshal(map[string]string{
		"user":         username,
		"password":     password,
		"notes":        notes,
		"email":        email,
		"phone_number": phone,
	})
	if b, err := signedInternalPost(endpoint, body); err != nil {
		log.Logger.Errorf("create-user fail user=%s err=%v body=%v", username, err, string(b))
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "创建用户失败，请联系管理员"})
		return
	}
	AuditLog("user-created user=%s ip=%s", username, c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "用户创建成功，请登录"})
}
