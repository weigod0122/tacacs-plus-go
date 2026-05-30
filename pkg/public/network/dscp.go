package network

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"
	"tacacs/pkg/public/log"
)

// rawConner 是「包装型 net.Conn」对外暴露底层连接的结构性约束。
// 典型实现:开启 PROXY protocol 后 Accept 返回的 *proxyproto.Conn —— 它用 net.Conn 接口
// 对外暴露,但握着真正的 *net.TCPConn,自身不直接提供 SyscallConn。
// 这里用 duck typing 而不是直接 import 具体包,避免 network 这个底层工具包反向依赖
// 上层用到的 proxy 库;任何提供 Raw() net.Conn 的类型都能被解包。
type rawConner interface {
	Raw() net.Conn
}

// SetDSCP 设置连接的 DSCP 标记
// dscp: DSCP 值 (0-63)，0 表示不设置
// conn: 网络连接
func SetDSCP(conn net.Conn, dscp string) error {
	if dscp == "" || dscp == "0" {
		return nil // 不设置 DSCP
	}

	dscpValue, err := strconv.Atoi(dscp)
	if err != nil {
		return fmt.Errorf("invalid DSCP value: %s, error: %v", dscp, err)
	}

	if dscpValue < 0 || dscpValue > 63 {
		return fmt.Errorf("DSCP value %d out of range (0-63)", dscpValue)
	}

	// 包装型连接(如 *proxyproto.Conn)只实现 net.Conn,需要先剥到底层 *net.TCPConn
	// 才能拿 syscall.RawConn 调 setsockopt。一层即可,目前没有多层包装的场景。
	if w, ok := conn.(rawConner); ok {
		conn = w.Raw()
	}

	// 检测连接类型和 IP 版本
	var (
		rawConn  syscall.RawConn
		isIPv6   bool
		connType string
	)

	switch c := conn.(type) {
	case *net.TCPConn:
		rawConn, err = c.SyscallConn()
		if err != nil {
			return fmt.Errorf("failed to get raw TCP connection: %v", err)
		}
		connType = "TCP"
		// 对于 TCP 连接，使用远程地址
		if remoteAddr := conn.RemoteAddr(); remoteAddr != nil {
			isIPv6 = isIPv6Address(remoteAddr.String())
		}
	case *net.UDPConn:
		rawConn, err = c.SyscallConn()
		if err != nil {
			return fmt.Errorf("failed to get raw UDP connection: %v", err)
		}
		connType = "UDP"
		// 对于 UDP 连接，优先使用远程地址，如果没有则使用本地地址
		if remoteAddr := conn.RemoteAddr(); remoteAddr != nil {
			isIPv6 = isIPv6Address(remoteAddr.String())
		} else if localAddr := conn.LocalAddr(); localAddr != nil {
			isIPv6 = isIPv6Address(localAddr.String())
		}
	default:
		return fmt.Errorf("unsupported connection type: %T, only TCP and UDP are supported", conn)
	}

	// 设置 DSCP
	var setErr error
	err = rawConn.Control(func(fd uintptr) {
		if isIPv6 {
			// IPv6 使用 IPV6_TCLASS
			// 对于 IPv6，DSCP 值直接设置到 Traffic Class 字段
			setErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, syscall.IPV6_TCLASS, dscpValue<<2)
		} else {
			// IPv4 使用 IP_TOS
			// DSCP 占用 IP 头部的 6 位，TOS 占用 8 位，TOS = DSCP << 2
			setErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_TOS, dscpValue<<2)
		}

		// 获取连接地址用于日志记录
		var connAddr string
		if remoteAddr := conn.RemoteAddr(); remoteAddr != nil {
			connAddr = remoteAddr.String()
		} else if localAddr := conn.LocalAddr(); localAddr != nil {
			connAddr = localAddr.String()
		} else {
			connAddr = "unknown"
		}

		if setErr != nil {
			log.Logger.Errorf("Failed to set DSCP %d for %s %s connection %s: %v",
				dscpValue, getIPVersion(isIPv6), connType, connAddr, setErr)
		}
	})

	if err != nil {
		return fmt.Errorf("failed to control raw connection: %v", err)
	}

	return setErr
}

// ValidateDSCP 验证 DSCP 值是否有效
func ValidateDSCP(dscp string) bool {
	if dscp == "" || dscp == "0" {
		return true // "0" 和空值表示不设置，是有效的
	}

	dscpValue, err := strconv.Atoi(dscp)
	if err != nil {
		return false
	}

	return dscpValue >= 0 && dscpValue <= 63
}

// isIPv6Address 判断连接地址是否为 IPv6
func isIPv6Address(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// 如果不是 host:port 格式，直接解析地址
		ip := net.ParseIP(addr)
		return ip != nil && ip.To4() == nil
	}

	ip := net.ParseIP(host)
	return ip != nil && ip.To4() == nil
}

// getIPVersion 获取 IP 版本字符串
func getIPVersion(isIPv6 bool) string {
	if isIPv6 {
		return "IPv6"
	}
	return "IPv4"
}

// CreateDSCPListener 创建一个预设置 DSCP 的监听器
// 这样可以确保包括三次握手在内的所有包都带有 DSCP 标记
func CreateDSCPListener(network, address, dscp string) (net.Listener, error) {
	if dscp == "" || dscp == "0" {
		// 如果不设置 DSCP，使用默认监听器
		return net.Listen(network, address)
	}

	dscpValue, err := strconv.Atoi(dscp)
	if err != nil {
		return nil, fmt.Errorf("invalid DSCP value '%s': %v", dscp, err)
	}

	if dscpValue < 0 || dscpValue > 63 {
		return nil, fmt.Errorf("DSCP value %d out of range (0-63)", dscpValue)
	}

	// 创建带有 Control 函数的 ListenConfig
	listenConfig := &net.ListenConfig{
		Control: func(net, addr string, c syscall.RawConn) error {
			var setErr error
			err := c.Control(func(fd uintptr) {
				// 判断是否为 IPv6
				isIPv6 := strings.Contains(network, "6")

				if isIPv6 {
					// IPv6 使用 IPV6_TCLASS
					setErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, syscall.IPV6_TCLASS, dscpValue<<2)
				} else {
					// IPv4 使用 IP_TOS
					setErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_TOS, dscpValue<<2)
				}

				if setErr != nil {
					log.Logger.Errorf("Failed to set DSCP %d on listener socket %s: %v (will use connection-level DSCP instead)",
						dscpValue, addr, setErr)
				} else {
					log.Logger.Infof("Successfully set DSCP %d on listener socket %s (%s)",
						dscpValue, addr, getIPVersion(isIPv6))
				}
			})

			// 不因为 DSCP 设置失败而阻止监听器创建
			// 监听器创建失败时才返回错误
			if err != nil {
				return fmt.Errorf("failed to control listener socket: %v", err)
			}
			// 即使 DSCP 设置失败，也不返回错误，让监听器正常创建
			return nil
		},
	}

	// 使用 ListenConfig 创建监听器
	listener, err := listenConfig.Listen(context.Background(), network, address)
	if err != nil {
		return nil, fmt.Errorf("failed to create DSCP listener: %v", err)
	}

	return listener, nil
}

// CreateDSCPDialer 创建一个预设置 DSCP 的 Dialer
// 用于客户端连接，确保三次握手包也带有 DSCP 标记
func CreateDSCPDialer(dscp string) (*net.Dialer, error) {
	if dscp == "" || dscp == "0" {
		return &net.Dialer{}, nil
	}

	dscpValue, err := strconv.Atoi(dscp)
	if err != nil {
		return nil, fmt.Errorf("invalid DSCP value '%s': %v", dscp, err)
	}

	if dscpValue < 0 || dscpValue > 63 {
		return nil, fmt.Errorf("DSCP value %d out of range (0-63)", dscpValue)
	}

	dialer := &net.Dialer{
		Control: func(network, address string, c syscall.RawConn) error {
			var setErr error
			err := c.Control(func(fd uintptr) {
				// 判断是否为 IPv6
				isIPv6 := strings.Contains(network, "6")

				if isIPv6 {
					// IPv6 使用 IPV6_TCLASS
					setErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, syscall.IPV6_TCLASS, dscpValue<<2)
				} else {
					// IPv4 使用 IP_TOS
					setErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_TOS, dscpValue<<2)
				}

				if setErr != nil {
					log.Logger.Errorf("Failed to set DSCP %d before connecting to %s: %v",
						dscpValue, address, setErr)
				} else {
					log.Logger.Infof("Successfully set DSCP %d before connecting to %s (%s)",
						dscpValue, address, getIPVersion(isIPv6))
				}
			})

			if err != nil {
				return fmt.Errorf("failed to control client socket: %v", err)
			}
			return setErr
		},
	}

	return dialer, nil
}
