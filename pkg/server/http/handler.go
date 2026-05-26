package http

import (
	"database/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"tacacs/pkg/public/cfg"
	"tacacs/pkg/public/db"
)

func httpApiHealth(c *gin.Context) {
	//数据库检查
	for _, dbTemp := range []*sql.DB{db.DbRead, db.DbWrite} {
		dbErr := dbTemp.Ping()
		if dbErr != nil {
			c.JSON(http.StatusFailedDependency, gin.H{
				"code":    http.StatusFailedDependency,
				"message": fmt.Sprintf("connect to db failed, err: %v", dbErr),
			})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "all service is healthy",
	})
	return
}

// httpApiMetaRefresh 给管理员手动触发缓存失效用：把 tacacs_meta 6 个 key 全部 +1，
// 无视触发器开关与是否真有数据变更，强制让 client 在下一次 2s 轮询时全量重建。
// 适用场景：DBA 绕过 server 直接改了 DB，需要立刻让缓存生效（否则要等 5min 兜底）。
func httpApiMetaRefresh(c *gin.Context) {
	if err := db.RefreshAllMeta(); err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("refresh tacacs_meta failed: %v", err),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "tacacs_meta version bumped for all keys",
	})
}

func setResponseHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := "*"
		if conf := cfg.ServerConfig(); conf != nil && conf.SwmAuth.AllowedOrigin != "" {
			origin = conf.SwmAuth.AllowedOrigin
		}
		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, PATCH, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-SwM-User, X-SwM-Is-Admin, X-SwM-Signature")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
