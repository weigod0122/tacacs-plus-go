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
