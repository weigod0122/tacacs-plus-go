package tac_plus

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"tacacs/pkg/public/cfg"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/network"
	"time"

	"github.com/pires/go-proxyproto"
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
			Secret:       []byte(cfg.ClientConfig().TacPlus.ShareKey),
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
				if dscp := cfg.ClientConfig().TacPlus.DSCP; dscp != "" && dscp != "0" {
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
	if dscp := cfg.ClientConfig().TacPlus.DSCP; dscp != "" {
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
	address := cfg.ClientConfig().TacPlus.IP + ":" + cfg.ClientConfig().TacPlus.Port
	dscp := cfg.ClientConfig().TacPlus.DSCP

	listener, err := network.CreateDSCPListener("tcp", address, dscp)
	if err != nil {
		return fmt.Errorf("failed to create DSCP listener on %v: %v", address, err)
	}
	// 可选启用 PROXY protocol:当 cfg.tacPlus.proxyTrustedCidrs 非空时,把 listener
	// 包一层,使来自可信代理(DPVS/HAProxy/Nginx Stream 等)的连接能解析 PROXY 头
	// 拿到真实交换机 IP;留空则不启用,RemoteAddr() 取 TCP 对端。
	// 失败路径下 wrapWithProxyProtocol 会自己关掉传入的 ln,这里不再 Close。
	listener, err = wrapWithProxyProtocol(listener, cfg.ClientConfig().TacPlus.ProxyTrustedCidrs)
	if err != nil {
		return fmt.Errorf("failed to enable PROXY protocol: %v", err)
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

// wrapWithProxyProtocol 把原 listener 包成识别 PROXY protocol 的 listener。
// trustedCidrs 来自 cfg.tacPlus.proxyTrustedCidrs(YAML 切片),空切片 = 不启用。
//
// 所有权契约:
//   - 成功路径返回的 net.Listener 接管原 ln(可能就是 ln 本身,也可能是包装后的)
//   - 失败路径会先关掉 ln 再返回 error,调用方不需要再 Close;若不关,fd 会泄漏
//
// 三种结果:
//   - trustedCidrs 为空 → 返回原 listener,不启用 PROXY 解析,RemoteAddr() 取 TCP 对端。
//   - trustedCidrs 解析失败 → 关闭 ln,返回 error,Start 中止。启动期快失败,避免错误配置在生产里静默退化成"任意来源都能伪造 IP"。
//   - trustedCidrs 解析成功 → 返回 *proxyproto.Listener,ConnPolicy 语义:
//   - 连接源 IP 落在 trustedCidrs 任一段内 → USE,解析 PROXY 头,RemoteAddr() 返回 PROXY 头里的原始客户端 IP
//   - 其它源(包括交换机直连)→ IGNORE,丢弃任何 PROXY 头不解析,RemoteAddr() 返回真实 TCP 对端;允许直连交换机继续工作
//
// 选 USE/IGNORE 而不是 REQUIRE/REJECT 的原因:让 LB 转发与交换机直连并存。
// 应用层只承担"防 PROXY 头伪造 IP"这一安全底线,源端访问控制(谁能连到 49端口)由网络侧防火墙负责。
//
// 用 ConnPolicy 而非 Policy:库已把 Policy 字段标记 Deprecated(只看 upstream),
// ConnPolicy 接 ConnPolicyOptions{Upstream, Downstream},未来若按本机入向接口分
// 策略也不用再改 API;两者互斥,同时设置库会在 Accept 路径 panic。
//
// 注意 ReadHeaderTimeout:不可信源进来后,库会先 peek 看是不是 PROXY 头;
// 若对端打开 TCP 但迟迟不发字节(慢速攻击),Accept 会被卡住。给个 5s 超时,
// 与上层 ReadTimeout 不冲突。
func wrapWithProxyProtocol(ln net.Listener, trustedCidrs []string) (net.Listener, error) {
	nets := make([]*net.IPNet, 0, len(trustedCidrs))
	for _, c := range trustedCidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			_ = ln.Close()
			return nil, fmt.Errorf("invalid CIDR %q in proxyTrustedCidrs: %v", c, err)
		}
		nets = append(nets, n)
	}
	if len(nets) == 0 {
		return ln, nil
	}
	log.Logger.Infof("PROXY protocol enabled, trusted CIDRs: %v", nets)
	return &proxyproto.Listener{
		Listener:          ln,
		ReadHeaderTimeout: 5 * time.Second,
		ConnPolicy: func(opts proxyproto.ConnPolicyOptions) (proxyproto.Policy, error) {
			host, _, err := net.SplitHostPort(opts.Upstream.String())
			if err != nil {
				return proxyproto.IGNORE, nil
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return proxyproto.IGNORE, nil
			}
			for _, n := range nets {
				if n.Contains(ip) {
					return proxyproto.USE, nil
				}
			}
			return proxyproto.IGNORE, nil
		},
	}, nil
}
