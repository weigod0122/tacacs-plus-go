package http

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"tacacs/pkg/public/cfg"
	"tacacs/pkg/public/log"

	"github.com/gin-gonic/gin"
)

// 豁免 IP 校验的路径（健康检查留给监控/LB,通常来自任意源）。
var ipWhitelistExemptPaths = map[string]struct{}{
	"/health": {},
}

// ipWhitelistMiddleware 在签名校验之前先按源 IP 兜底拒绝。
// 配置走 cfg.ServerConfig().SwmAuth.AllowedCIDRs;为空时回退到 loopback。
// 工厂阶段 CIDR 非法直接 panic —— 启动期暴露问题,好过运行期一边静默放行一边日志报错。
func ipWhitelistMiddleware() gin.HandlerFunc {
	nets, err := cfg.ServerConfig().SwmAuth.ResolveAllowedNets()
	if err != nil {
		panic(fmt.Errorf("invalid swm_auth.allowed_cidrs: %w", err))
	}
	log.Logger.Infof("server IP whitelist enabled: %s", cidrsToString(nets))

	return func(c *gin.Context) {
		if _, ok := ipWhitelistExemptPaths[c.Request.URL.Path]; ok {
			c.Next()
			return
		}

		ipStr := c.ClientIP()
		ip := net.ParseIP(ipStr)
		if ip == nil {
			AuditLog("ip-whitelist-reject reason=parse-fail src=%s path=%s method=%s",
				ipStr, c.Request.URL.Path, c.Request.Method)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "ip not allowed"})
			return
		}

		for _, n := range nets {
			if n.Contains(ip) {
				c.Next()
				return
			}
		}

		AuditLog("ip-whitelist-reject src=%s path=%s method=%s",
			ipStr, c.Request.URL.Path, c.Request.Method)
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "ip not allowed"})
	}
}

func cidrsToString(nets []*net.IPNet) string {
	parts := make([]string, 0, len(nets))
	for _, n := range nets {
		parts = append(parts, n.String())
	}
	return strings.Join(parts, ", ")
}
