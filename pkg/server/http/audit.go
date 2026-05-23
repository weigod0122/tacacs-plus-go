package http

import (
	"fmt"

	"tacacs/pkg/server/internal/slog"
)

func AuditLog(format string, args ...any) {
	slog.AuditLogger.Info(fmt.Sprintf(format, args...))
}
