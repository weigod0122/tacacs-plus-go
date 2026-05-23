// Package client 仅作为 cmd/client 的入口外观（facade），把 client 进程在 main
// 中需要的初始化动作转发到 pkg/client/internal/* 下的实现。
package client

import (
	"tacacs/pkg/client/internal/clog"
)

// InitLog 初始化 client 专属日志器（tac_plus_account / tac_plus_authen / tac_plus_author）。
// 必须在 utils.InitAppLog 之后调用，logPath 与 utils.InitAppLog 传入的相同。
func InitLog(logPath string) {
	clog.Init(logPath)
}
