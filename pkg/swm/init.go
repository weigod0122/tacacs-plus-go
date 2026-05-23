// Package swm 仅作为 cmd/swm 的入口外观（facade），把 swm 进程在 main 中需要
// 的初始化动作转发到 pkg/swm/internal/* 下的实现。
package swm

import (
	"tacacs/pkg/swm/internal/swlog"
)

// InitLog 初始化 swm 专属日志器（audit）。
// 必须在 utils.InitAppLog 之后调用，logPath 与 utils.InitAppLog 传入的相同。
func InitLog(logPath string) {
	swlog.Init(logPath)
}
