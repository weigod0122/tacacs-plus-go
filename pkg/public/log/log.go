package log

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync/atomic"

	gologger "github.com/weigod0122/go-logger"
)

// Logger 是所有 item（server/client/swm）共用的应用日志，
// 按 item 落到 logPath/{item}_app.log（server_app.log / client_app.log / swm_app.log）。
// 三服务共用同一个 log_file_path 目录时，文件名前缀防止互相覆盖。
// item 专属的 logger 放在各自的 internal/{slog,clog,swlog} 子包下，跨 item import 会被
// Go 编译器拒绝，避免误用。
var (
	Logger      = gologger.NewLogger()
	DebugLogger = gologger.NewLogger()
)

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

	_ = DebugLogger.Detach("console")
	_ = DebugLogger.Attach("file", gologger.LOGGER_LEVEL_DEBUG, &gologger.FileConfig{
		Filename:  fmt.Sprintf("%v/%v_debug.log", logPath, item),
		DateSlice: "d",
		// DebugLog 自己用 runtime.Caller 把真实调用点拼进 body,这里不再用
		// gologger 的 %file%:%line%——否则永远显示 log.go:DebugLog 那一行。
		Format: "%millisecond_format% [%level_string%] %body%",
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

// debugFlag 是 DebugLog 的开关。`log` 不再反向 import `pkg/public/env`(会跟
// env 形成循环依赖),由 env 在初始化与 Apollo 热更新回调里单向调用 SetDebug
// 把状态写过来。原子操作是必须的:OnChange 在 agollo 自己的 goroutine 触发,
// DebugLog 又在请求路径上高频读。
var debugFlag atomic.Bool

// SetDebug 由 env 包(唯一写入方)调用,把 env.DEBUG 同步到 log 包。
func SetDebug(v bool) { debugFlag.Store(v) }

// DebugLog 在 DEBUG 开关打开时往 DebugLogger 写一条记录。
// gologger.Writer 内部写死 runtime.Caller(2),透过本函数包装一层后它只能拿到
// log.go 的位置,所以这里自己 Caller(1) 取真实调用点拼到 body 前面;
// 配套地,InitAppLog 里 DebugLogger 的 Format 已经移除 [%file%:%line%],避免一行
// 出现一个错位置 + 一个正确位置。
func DebugLog(msg string, v ...interface{}) {
	if !debugFlag.Load() {
		return
	}
	go func() {
		_, file, line, ok := runtime.Caller(1)
		if !ok {
			file = "?"
		} else if i := strings.LastIndex(file, "/"); i >= 0 {
			file = file[i+1:]
		}
		args := make([]interface{}, 0, len(v)+2)
		args = append(args, file, line)
		args = append(args, v...)
		DebugLogger.Infof("[%s:%d] "+msg, args...)
	}()
}
