package http

import "github.com/gin-gonic/gin"

func configRoutes(app *gin.Engine) {
	//系统健康检查
	app.GET("/health", httpApiHealth)
}
