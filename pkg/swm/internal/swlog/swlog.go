// Package swlog 是 swm 专属的日志器集合。
//
// 通过放在 pkg/swm/internal/ 下，Go 编译器只允许 pkg/swm 子树和 cmd/swm import
// 本包，server/client 误 import 会编译报错。
//
// 注意：server 自己也有 audit logger（pkg/server/internal/slog.AuditLogger 写
// server_audit.log），swm 写 swm_audit.log。两个进程通常分开部署，即便共享同一个
// log_file_path 目录（典型的"同机部署"场景）也不会撞文件名。
package swlog

import (
	"fmt"
	"tacacs/pkg/public/log"

	gologger "github.com/weigod0122/go-logger"
)

// AuditLogger 写 swm_audit.log：登录成功/失败、登出、管理员写操作、CSRF 校验失败、未授权访问等。
var AuditLogger = gologger.NewLogger()

// Init 把 swm 专属 logger 落到 logPath 目录。logPath 由 utils.InitAppLog 已经创建过。
func Init(logPath string) {
	logPath = log.NormalizeLogPath(logPath)

	_ = AuditLogger.Detach("console")
	_ = AuditLogger.Attach("file", gologger.LOGGER_LEVEL_INFO, &gologger.FileConfig{
		Filename:  fmt.Sprintf("%v/swm_audit.log", logPath),
		DateSlice: "d",
		Format:    "%millisecond_format% %body%",
	})
}
