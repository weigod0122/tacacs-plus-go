// Package server 仅作为 cmd/server 的入口外观（facade），把 server 进程在 main
// 中需要的初始化动作转发到 pkg/server/internal/* 下的实现。
//
// 通过这个外观，cmd/server 不需要 import internal 包（Go 编译器会拒绝），仍然能
// 触发 internal 包的初始化逻辑。同时 client/swm 想直接访问 server 的内部 logger
// 也走不通。
package server

import (
	"tacacs/pkg/server/internal/slog"
)

// InitLog 初始化 server 专属日志器（audit、http_api）。
// 必须在 utils.InitAppLog 之后调用，logPath 与 utils.InitAppLog 传入的相同。
func InitLog(logPath string) {
	slog.Init(logPath)
}
