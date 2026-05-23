package http

import (
	"fmt"
	"net"
	"net/http"
	"tacacs/pkg/public/cfg"
	"tacacs/pkg/public/db"
	"time"

	"github.com/gin-gonic/gin"
)

// httpApiHealth 是运维探活入口。两步检查任一失败即视为不健康:
//  1. Ping 只读库:client 业务热路径要靠它读 role/server/cmd 模板;
//  2. Dial TACACS+ 监听端口:Listener 死了的话设备来连接会被拒,
//     而 client 进程本身可能还在跑,只 ping DB 检测不出来。
func httpApiHealth(c *gin.Context) {
	if db.DbRead == nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": "DbRead is not initialized",
		})
		return
	}
	if err := db.DbRead.Ping(); err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("connect to read db failed, err: %v", err),
		})
		return
	}

	if err := probeTacPlusListener(); err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("tacacs+ listener not reachable: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "service is ok",
	})
}

// probeTacPlusListener 用 1 秒超时的 TCP dial 验证 TACACS+ 监听端口能接受新连接。
// 监听 ip 是 0.0.0.0 / :: 这类未指定地址时,改成 127.0.0.1 / ::1 探本机即可。
func probeTacPlusListener() error {
	tp := cfg.ClientConfig().TacPlus
	host := tp["ip"]
	port := tp["port"]
	if port == "" {
		return fmt.Errorf("tacPlus.port not configured")
	}
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::", "[::]":
		host = "::1"
	}
	addr := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}
