package http

import (
	"fmt"

	"tacacs/pkg/swm/internal/swlog"
)

// AuditLog 写入审计日志（独立文件 swm_audit.log，由 swlog.AuditLogger 写入）。
// 用于：登录成功/失败、登出、管理员的写操作、未授权访问尝试。
func AuditLog(format string, args ...any) {
	swlog.AuditLogger.Info(fmt.Sprintf(format, args...))
}
