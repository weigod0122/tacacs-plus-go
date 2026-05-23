package log

import (
	"fmt"
	"os"
	"strings"

	gologger "github.com/weigod0122/go-logger"
)

// Logger 是所有 item（server/client/swm）共用的应用日志，
// 按 item 落到 logPath/{item}_app.log（server_app.log / client_app.log / swm_app.log）。
// 三服务共用同一个 log_file_path 目录时，文件名前缀防止互相覆盖。
// item 专属的 logger 放在各自的 internal/{slog,clog,swlog} 子包下，跨 item import 会被
// Go 编译器拒绝，避免误用。
var Logger = gologger.NewLogger()

// InitAppLog 创建日志目录并把 Logger 落到 logPath/{item}_app.log。
// item 透传自上层 cmd 入口（"server" / "client" / "swm"）；
// 不复用 env.Item 是为了避免 pkg/public/log 反向依赖 pkg/public/env（env 已经依赖 log）。
// 各 item 自己的 logger 包（slog/clog/swlog）也会用同一个 logPath 写自己的文件，
// 但目录创建只在这里做一次。
func InitAppLog(item, logPath string) error {
	logPath = NormalizeLogPath(logPath)
	if err := os.MkdirAll(logPath, os.ModePerm); err != nil {
		return fmt.Errorf("mkdir log path failed: %v", err)
	}

	_ = Logger.Detach("console")
	_ = Logger.Attach("file", gologger.LOGGER_LEVEL_DEBUG, &gologger.FileConfig{
		Filename:  fmt.Sprintf("%v/%v_app.log", logPath, item),
		DateSlice: "d",
		Format:    "%millisecond_format% [%level_string%] [%file%:%line%] %body%",
	})
	return nil
}

// NormalizeLogPath 给 item 专属 logger 用：空路径回退 ./log/，去掉末尾斜杠。
func NormalizeLogPath(logPath string) string {
	if logPath == "" {
		logPath = "./log/"
	}
	return strings.TrimRight(logPath, "/")
}
