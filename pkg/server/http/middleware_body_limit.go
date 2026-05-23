package http

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// maxBodyBytes 是 Server 接受的最大请求体。
// 1MB 远大于任何业务接口正常需要的载荷,触顶基本是脚本误用、前端 bug 或攻击。
// 真实业务里最大的是"模板批量录入"(命令/服务器列表),单次几千条也远不到 1MB。
const maxBodyBytes = 1 << 20

// bodyTooLargeSuggestion 是 413 响应里给前端的可读建议。
// 直接拼到 msg 字段——SwM 前端的 toast 只取 data.msg/data.message,
// 不会读其他自定义字段,把建议嵌进 msg 才能真正显示给用户。
const bodyTooLargeSuggestion = "请求体超过 1MB 上限。建议:" +
	"① 拆成多次提交;" +
	"② 用「命令模板/服务器模板/角色模板」预先封装大批量配置," +
	"后续按模板引用而非一次性平铺所有明细。"

// bodyLimitMiddleware 把 Request.Body 套上 1MB 上限。
// 实际读到第 (maxBodyBytes+1) 字节时 MaxBytesReader 报错,由后续
// httpApiLog 中间件的 ReadAll 分支捕获,转成 413 响应 + 拆分建议。
// 之所以不在这里做 Content-Length 预判 + 早期 abort,是因为这样能让
// 拒绝路径走 httpApiLog,留下统一的访问日志,便于 ops 排查。
func bodyLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
		c.Next()
	}
}

// respondBodyTooLarge 返回标准 413 响应,并落一条审计日志。
// 同时设置 code/message 两个字段是为了兼容仓库里两种历史 JSON 风格:
//   - 多数中间件用 {code, msg}
//   - 部分 handler 用 {code, message}
//
// 双写一份,前端拿哪个字段都能拿到提示。
func respondBodyTooLarge(c *gin.Context) {
	AuditLog("body-too-large declared=%d limit=%d src=%s path=%s method=%s",
		c.Request.ContentLength, maxBodyBytes, c.ClientIP(),
		c.Request.URL.Path, c.Request.Method)
	body := gin.H{
		"code":    http.StatusRequestEntityTooLarge,
		"msg":     fmt.Sprintf("请求体过大(上限 %d 字节)。%s", maxBodyBytes, bodyTooLargeSuggestion),
		"message": fmt.Sprintf("请求体过大(上限 %d 字节)。%s", maxBodyBytes, bodyTooLargeSuggestion),
	}
	c.JSON(http.StatusRequestEntityTooLarge, body)
}

// isBodyTooLargeErr 判别 ReadAll 错误是否由 MaxBytesReader 触发。
// 给 httpApiLog 的 ReadAll 失败分支用。
func isBodyTooLargeErr(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}
