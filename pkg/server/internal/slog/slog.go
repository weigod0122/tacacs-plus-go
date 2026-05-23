// Package slog 是 server 专属的日志器集合。
//
// 通过放在 pkg/server/internal/ 下，Go 编译器只允许 pkg/server 子树和 cmd/server
// import 本包，client/swm 误 import 会编译报错。
package slog

import (
	"fmt"
	"tacacs/pkg/public/log"

	gologger "github.com/weigod0122/go-logger"
)

// AuditLogger 写 server_audit.log：管理员写操作、签名校验失败等审计事件。
// 文件名带 server_ 前缀,与 swm 的 swm_audit.log 区分,共享同一个 log_file_path 时不会撞名。
var AuditLogger = gologger.NewLogger()

// HttpApiLogger 写 server_http_api.log：HTTP 访问日志（gin 中间件）。
var HttpApiLogger = gologger.NewLogger()

// Init 把 server 专属 logger 落到 logPath 目录。logPath 由 utils.InitAppLog 已经创建过。
func Init(logPath string) {
	logPath = log.NormalizeLogPath(logPath)

	_ = AuditLogger.Detach("console")
	_ = AuditLogger.Attach("file", gologger.LOGGER_LEVEL_INFO, &gologger.FileConfig{
		Filename:  fmt.Sprintf("%v/server_audit.log", logPath),
		DateSlice: "d",
		Format:    "%millisecond_format% %body%",
	})

	_ = HttpApiLogger.Detach("console")
	_ = HttpApiLogger.Attach("file", gologger.LOGGER_LEVEL_INFO, &gologger.FileConfig{
		Filename:  fmt.Sprintf("%v/server_http_api.log", logPath),
		DateSlice: "d",
		Format:    "[GIN] %timestamp_format% %body%",
	})
}
