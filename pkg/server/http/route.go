package http

import (
	"github.com/gin-gonic/gin"
)

func configRoutes(app *gin.Engine) {
	//解决前端跨域问题
	app.Use(setResponseHeaders())

	//源 IP 白名单：默认仅放行 loopback,跨机部署需在 swm_auth.allowed_cidrs 显式声明
	app.Use(ipWhitelistMiddleware())

	//SwM 共享密钥签名校验 + 路径/body 级 ACL（OPTIONS 已被 setResponseHeaders 短路）
	app.Use(swmAuthMiddleware())

	//系统健康检查
	app.GET("/health", httpApiHealth)

	//用户管理接口(不包含角色设置，只有基础信息：用户名、密码、备注)
	user := app.Group("/tacacs/user")
	user.GET("/get", httpApiUserGet)
	user.GET("/get-admin", httpApiUserGetAdmin)
	user.POST("/create", httpApiUserCreate)
	user.POST("/check", httpApiCheckUser)
	user.POST("/update/password", httpApiUserUpdatePassword)
	user.POST("/update/notes", httpApiUserUpdateNotes)
	user.POST("/update/basicInfo", httpApiUserUpdateBasicInfo)
	user.POST("/reset/password", httpApiUserResetPassword)
	user.DELETE("/delete", httpApiUserDelete)
	user.GET("/clear/updatePasswordErrMap", httpClearUpdatePasswordErrUser)
	user.GET("/clear/checkPasswordErrMap", httpClearCheckPasswordErrUser)

	//命令模版管理接口
	templateCmd := app.Group("/tacacs/template/command")
	templateCmd.GET("/get", httpApiTemplateCmdGet)
	templateCmd.POST("/add", httpApiTemplateCmdAdd)
	templateCmd.DELETE("/delete", httpApiTemplateCmdDelete)

	//服务器模版管理接口
	templateServer := app.Group("/tacacs/template/server")
	templateServer.GET("/get", httpApiTemplateServerGet)
	templateServer.POST("/add", httpApiTemplateServerAdd)
	templateServer.DELETE("/delete", httpApiTemplateServerDelete)

	//角色定义接口
	role := app.Group("/tacacs/template/role")
	role.GET("/get", httpApiRoleGet)
	role.POST("/create", httpApiRoleCreate)
	role.DELETE("/delete", httpApiRoleDelete)

	//审批系统接口
	approval := app.Group("/tacacs/approval")
	approval.GET("/get", httpApiApprovalGet)        //获取当前审批工单
	approval.POST("/create", httpApiApprovalCreate) //创建审批工单
	approval.POST("/update", httpApiApprovalUpdate) //审批选定工单

	//缓存元数据接口（仅管理员）：强制让 client 全量重建本地缓存
	app.POST("/tacacs/meta/refresh", httpApiMetaRefresh)

}
