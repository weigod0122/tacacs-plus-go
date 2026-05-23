package http

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// 简单 IP 限流：每个 IP 一个令牌桶，固定窗口（默认 1 分钟 5 次）。
// 失败 N 次后冷却 lockoutDur。专用于 /login 这类高敏感入口。

const (
	loginAttemptWindow = time.Minute
	loginAttemptMax    = 5
	lockoutDur         = 15 * time.Minute
	cleanupInterval    = 5 * time.Minute
)

type ipBucket struct {
	count       int
	windowStart time.Time
	failCount   int
	lockedUntil time.Time
}

var (
	ipBucketsMu sync.Mutex
	ipBuckets   = map[string]*ipBucket{}
	cleanupOnce sync.Once
)

func startCleanupOnce() {
	cleanupOnce.Do(func() {
		go func() {
			t := time.NewTicker(cleanupInterval)
			defer t.Stop()
			for range t.C {
				now := time.Now()
				ipBucketsMu.Lock()
				for ip, b := range ipBuckets {
					if now.Sub(b.windowStart) > 2*loginAttemptWindow && now.After(b.lockedUntil) {
						delete(ipBuckets, ip)
					}
				}
				ipBucketsMu.Unlock()
			}
		}()
	})
}

// LoginRateLimit 限流中间件：超出阈值返回 429。
// 在 handler 里调用 RecordLoginFailure(c) 累计失败次数；成功登录后调用 ResetLoginCounter(c)。
func LoginRateLimit() gin.HandlerFunc {
	startCleanupOnce()
	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()

		ipBucketsMu.Lock()
		b, ok := ipBuckets[ip]
		if !ok {
			b = &ipBucket{windowStart: now}
			ipBuckets[ip] = b
		}
		if now.Before(b.lockedUntil) {
			ipBucketsMu.Unlock()
			AuditLog("login-locked ip=%s until=%s", ip, b.lockedUntil.Format(time.RFC3339))
			respondRateLimited(c, "登录尝试次数过多，账户已暂时锁定，请稍后再试")
			c.Abort()
			return
		}
		if now.Sub(b.windowStart) > loginAttemptWindow {
			b.windowStart = now
			b.count = 0
		}
		b.count++
		over := b.count > loginAttemptMax
		ipBucketsMu.Unlock()

		if over {
			AuditLog("login-throttle ip=%s count=%d", ip, b.count)
			respondRateLimited(c, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}
		c.Next()
	}
}

// respondRateLimited 渲染合适的 429 响应：
//   - /login 表单提交 → 渲染登录页（带内联红色错误 + 触发 JS toast 警示弹窗）
//   - 其他（XHR/JSON）→ 返回结构体
func respondRateLimited(c *gin.Context, msg string) {
	if c.Request.URL.Path == "/login" && c.Request.Method == http.MethodPost {
		c.HTML(http.StatusTooManyRequests, "login.html", gin.H{
			"error":       msg,
			"rateLimited": true,
		})
		return
	}
	c.JSON(http.StatusTooManyRequests, gin.H{"code": 429, "msg": msg})
}

// RecordLoginFailure 累计 IP 的失败次数，超阈值则锁定。
func RecordLoginFailure(c *gin.Context) {
	ip := c.ClientIP()
	ipBucketsMu.Lock()
	defer ipBucketsMu.Unlock()
	b, ok := ipBuckets[ip]
	if !ok {
		b = &ipBucket{windowStart: time.Now()}
		ipBuckets[ip] = b
	}
	b.failCount++
	if b.failCount >= loginAttemptMax {
		b.lockedUntil = time.Now().Add(lockoutDur)
		b.failCount = 0
		AuditLog("login-lockout ip=%s for=%s", ip, lockoutDur)
	}
}

// ResetLoginCounter 登录成功后调用，清零失败计数。
func ResetLoginCounter(c *gin.Context) {
	ip := c.ClientIP()
	ipBucketsMu.Lock()
	defer ipBucketsMu.Unlock()
	if b, ok := ipBuckets[ip]; ok {
		b.failCount = 0
		b.lockedUntil = time.Time{}
	}
}
