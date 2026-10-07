package http

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Login and registration are bounded by source, account, and a bounded
// process-wide bucket. IP-only limits are easy to evade with many addresses,
// while account-only limits let one attacker lock out a victim.
const (
	loginAttemptWindow = time.Minute
	loginAttemptMax    = 5
	globalAttemptMax   = 1000
	lockoutDur         = 15 * time.Minute
	cleanupInterval    = 5 * time.Minute
	maxRateBuckets     = 100000
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
				for key, b := range ipBuckets {
					if now.Sub(b.windowStart) > 2*loginAttemptWindow && now.After(b.lockedUntil) {
						delete(ipBuckets, key)
					}
				}
				ipBucketsMu.Unlock()
			}
		}()
	})
}

// LoginRateLimit limits both /login and /create-user. The account hint is
// extracted without consuming the request body, so the handler sees the same
// form data. Non-form callers still receive source and global limits.
func LoginRateLimit() gin.HandlerFunc {
	startCleanupOnce()
	return func(c *gin.Context) {
		keys := rateLimitKeys(c)
		now := time.Now()

		ipBucketsMu.Lock()
		locked := false
		for _, key := range keys {
			b := getBucketLocked(key, now)
			if b != nil && now.Before(b.lockedUntil) {
				locked = true
				break
			}
		}
		if !locked {
			for _, key := range keys {
				if b := getBucketLocked(key, now); b != nil {
					b.count++
					limit := loginAttemptMax
					if key == "global" {
						limit = globalAttemptMax
					}
					if b.count > limit {
						locked = true
					}
				}
			}
		}
		ipBucketsMu.Unlock()

		if locked {
			AuditLog("login-throttle ip=%s account=%s", c.ClientIP(), requestUsername(c))
			respondRateLimited(c, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}
		c.Next()
	}
}

func getBucketLocked(key string, now time.Time) *ipBucket {
	if key == "" {
		return nil
	}
	if b, ok := ipBuckets[key]; ok {
		if now.Sub(b.windowStart) > loginAttemptWindow {
			b.windowStart = now
			b.count = 0
			// A failed-login lockout intentionally lasts longer than the
			// request window. Do not clear it at the first minute boundary.
			if !now.Before(b.lockedUntil) {
				b.failCount = 0
				b.lockedUntil = time.Time{}
			}
		}
		return b
	}
	if len(ipBuckets) >= maxRateBuckets {
		// Remove expired entries first. If an attacker is still filling the map,
		// refuse to allocate another bucket and rely on the bounded global bucket.
		for k, b := range ipBuckets {
			if now.After(b.lockedUntil) && now.Sub(b.windowStart) > 2*loginAttemptWindow {
				delete(ipBuckets, k)
			}
		}
		if len(ipBuckets) >= maxRateBuckets {
			return nil
		}
	}
	b := &ipBucket{windowStart: now}
	ipBuckets[key] = b
	return b
}

func rateLimitKeys(c *gin.Context) []string {
	keys := []string{"ip:" + strings.TrimSpace(c.ClientIP()), "global"}
	if username := requestUsername(c); username != "" {
		keys = append(keys, "user:"+strings.ToLower(username))
		c.Set("rate-limit-username", username)
	}
	return keys
}

func requestUsername(c *gin.Context) string {
	if value, ok := c.Get("rate-limit-username"); ok {
		if username, ok := value.(string); ok {
			return strings.TrimSpace(username)
		}
	}
	if value := strings.TrimSpace(c.Request.URL.Query().Get("username")); value != "" {
		return value
	}
	if c.Request.Body == nil || c.Request.Method == http.MethodGet {
		return ""
	}
	// Avoid reading only a prefix: restoring a prefix would silently truncate
	// the form body seen by the handler. Normal login/registration forms carry
	// Content-Length well below this threshold; leave larger/unknown bodies to
	// the IP and global buckets and let the body-limit middleware enforce size.
	// Unknown-length/chunked requests are deliberately left untouched.  The
	// public body-limit middleware will enforce the total size, while this
	// middleware must never consume a chunked body just to derive an account
	// key.
	if c.Request.ContentLength <= 0 || c.Request.ContentLength > 8192 {
		return ""
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		// Restore the bounded prefix so a later body-limit/parser error is still
		// handled by the normal request path rather than by an unexpected EOF.
		c.Request.Body = io.NopCloser(bytes.NewReader(raw))
		return ""
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	if len(raw) > 8192 {
		return ""
	}
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(values.Get("username"))
}

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

// RecordLoginFailure increments source, account and global failure counters.
func RecordLoginFailure(c *gin.Context) {
	keys := rateLimitKeys(c)
	now := time.Now()
	ipBucketsMu.Lock()
	defer ipBucketsMu.Unlock()
	for _, key := range keys {
		b := getBucketLocked(key, now)
		if b == nil {
			continue
		}
		b.failCount++
		limit := loginAttemptMax
		if key == "global" {
			limit = globalAttemptMax
		}
		if b.failCount >= limit {
			b.lockedUntil = now.Add(lockoutDur)
			b.failCount = 0
			AuditLog("login-lockout key=%s for=%s", key, lockoutDur)
		}
	}
}

// ResetLoginCounter clears source and account failure state. The global bucket
// remains a request-rate guard and is not reset by a successful login.
func ResetLoginCounter(c *gin.Context) {
	keys := rateLimitKeys(c)
	ipBucketsMu.Lock()
	defer ipBucketsMu.Unlock()
	for _, key := range keys {
		if key == "global" {
			continue
		}
		if b, ok := ipBuckets[key]; ok {
			b.failCount = 0
			b.lockedUntil = time.Time{}
		}
	}
}
