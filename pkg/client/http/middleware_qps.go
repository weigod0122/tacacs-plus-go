package http

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// QPS 上限。Client HTTP 只承担运维探活,正常 1 个监控源每 5s 探一次足够,
// 设 2 已经留了 4 倍冗余,任何超出都明显属于异常或攻击,直接 429。
const (
	httpQPSLimit  = 2
	httpQPSBurst  = 2
	tokenInterval = time.Second / httpQPSLimit
)

type tokenBucket struct {
	mu       sync.Mutex
	tokens   float64
	lastFill time.Time
}

var bucket = &tokenBucket{
	tokens:   httpQPSBurst,
	lastFill: time.Now(),
}

// allow 按 1 桶全局计量。Client HTTP 暴露面就是 /health,
// 无需按 IP 分桶,全局一个桶最简单也最贴合"只允许 2 QPS"的诉求。
func (b *tokenBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastFill).Seconds()
	b.tokens += elapsed * httpQPSLimit
	if b.tokens > httpQPSBurst {
		b.tokens = httpQPSBurst
	}
	b.lastFill = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// qpsLimitMiddleware 在所有路由(含 /health)前生效。
// 超限返回 429 + 简短 msg,挂到中间件链最前,避免拒绝路径再去访问 DB 或 dial TACACS+。
func qpsLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !bucket.allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code": http.StatusTooManyRequests,
				"msg":  "请求过于频繁(client HTTP 限速 2 QPS)",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
