package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// The SWM endpoints accept small forms and JSON requests. Keep the public
// entry point bounded even when it is deployed in front of a server that has
// its own body limit; this prevents a large request from being buffered by
// the reverse proxy before it reaches the server.
const maxSWMBodyBytes = 1 << 20

func requestBodyLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSWMBodyBytes)
		c.Next()
	}
}
