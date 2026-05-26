package tac_plus

import (
	"fmt"
	"net"
	"sync"
	"tacacs/pkg/public/cfg"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/network"
	"time"

	"github.com/robfig/cron/v3"

	//"github.com/nwaples/tacplus"
	"tacacs/pkg/public/tacplus"
)

type TacPlusSystem struct {
	server   tacplus.Server
	mutex    sync.RWMutex
	listener net.Listener
	handler  *myHandler
	cron     *cron.Cron
}

func NewTacPlusSystem() *TacPlusSystem {
	// 创建TACACS+服务器实例
	var handler = &myHandler{}
	serverConnHandler := tacplus.ServerConnHandler{
		Handler: handler,
		ConnConfig: tacplus.ConnConfig{
			Secret:       []byte(cfg.ClientConfig().TacPlus["shareKey"]),
			Log:          func(v ...interface{}) { log.Logger.Infof("TACACS+ server log:%v", v) },
			LegacyMux:    true,             // 🔥 启用Legacy Mux模式以支持连接复用
			ReadTimeout:  5 * time.Minute,  // 设置读超时为5分钟
			WriteTimeout: 5 * time.Minute,  // 设置写超时为5分钟
			IdleTimeout:  30 * time.Minute, // 设置空闲超时为30分钟（更长的长连接）
			Mux:          true,             // 🔥 启用连接复用，配合panic恢复机制使用
		},
	}
	tps := &TacPlusSystem{
		handler: handler,
		server: tacplus.Server{
			ServeConn: func(conn net.Conn) {
				defer func() {
					if r := recover(); r != nil {
						remoteAddr := func() string {
							defer func() { recover() }()
							if conn != nil {
								if addr := conn.RemoteAddr(); addr != nil {
									return addr.String()
								}
							}
							return "unknown"
						}()

						localAddr := func() string {
							defer func() { recover() }()
							if conn != nil {
								if addr := conn.LocalAddr(); addr != nil {
									return addr.String()
								}
							}
							return "unknown"
						}()
						log.Logger.Errorf("TACACS+ server running has panic:%v, remoteAddr:%s, localAddr:%s", r, remoteAddr, localAddr)
					}
				}()
				// 备用 DSCP 设置（监听器级别已设置，这里作为双重保险）
				// 注意：监听器级别的设置会应用到三次握手包，这里的设置只应用到数据包
				if dscp := cfg.ClientConfig().TacPlus["dscp"]; dscp != "" && dscp != "0" {
					if err := network.SetDSCP(conn, dscp); err != nil {
						log.Logger.Errorf("Failed to set backup DSCP for connection %s: %v", conn.RemoteAddr().String(), err)
					}
				}
				serverConnHandler.Serve(conn)
			},
			Log: func(i ...interface{}) { log.Logger.Errorf("TACACS+ server log:%v", i) },
		},
		cron: cron.New(cron.WithSeconds()),
	}
	return tps

}

func (tps *TacPlusSystem) Start() error {
	// 验证 DSCP 配置
	if dscp := cfg.ClientConfig().TacPlus["dscp"]; dscp != "" {
		if !network.ValidateDSCP(dscp) {
			return fmt.Errorf("invalid DSCP value: %s, must be between 0-63", dscp)
		}
		if dscp != "0" {
			log.Logger.Infof("DSCP marking enabled with value: %s", dscp)
		}
	}

	updateUsers()

	var tpsCronAddFuncErr error

	_, tpsCronAddFuncErr = tps.cron.AddFunc("*/2 * * * * *", updateUsers)
	if tpsCronAddFuncErr != nil {
		return fmt.Errorf("updateUsers定时任务添加失败：%v", tpsCronAddFuncErr)
	}

	_, tpsCronAddFuncErr = tps.cron.AddFunc("0 */5 * * * *", clearTempTacacsTableUpdateTime)
	if tpsCronAddFuncErr != nil {
		return fmt.Errorf("clearTempTacacsTableUpdateTime定时任务添加失败：%v", tpsCronAddFuncErr)
	}

	tps.cron.Start()

	// 监听TACACS+默认端口49，使用 DSCP 标记
	address := cfg.ClientConfig().TacPlus["ip"] + ":" + cfg.ClientConfig().TacPlus["port"]
	dscp := cfg.ClientConfig().TacPlus["dscp"]

	listener, err := network.CreateDSCPListener("tcp", address, dscp)
	if err != nil {
		return fmt.Errorf("failed to create DSCP listener on %v: %v", address, err)
	}
	tps.listener = listener

	// 启动服务器
	go func() {
		err = tps.server.Serve(tps.listener)
		if err != nil {
			log.Logger.Errorf("TACACS+ server running has err:%v", err)
		}
	}()
	return nil
}

func (tps *TacPlusSystem) Stop() {
	// 启动服务器
	log.Logger.Info("TACACS+ server stop")
	_ = tps.listener.Close()
	tps.cron.Stop()
}
