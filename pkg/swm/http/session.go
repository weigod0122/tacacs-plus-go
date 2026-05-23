package http

import (
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"sync"
	"tacacs/pkg/public/log"
)

const sessionKeyFile = "./session.key"

var (
	sessionSecretOnce sync.Once
	sessionSecret     []byte
)

// loadOrCreateSessionKey 读取本地持久化的 32 字节随机 key；不存在则生成并写入。
// 持久化保证服务重启后不会让所有用户被强制下线，又消除了硬编码 secret。
func loadOrCreateSessionKey() []byte {
	sessionSecretOnce.Do(func() {
		if data, err := os.ReadFile(sessionKeyFile); err == nil && len(data) >= 32 {
			sessionSecret = data[:32]
			return
		}
		buf := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, buf); err != nil {
			log.Logger.Errorf("generate session key fail: %v", err)
			os.Exit(1)
		}
		if err := os.MkdirAll(filepath.Dir(sessionKeyFile), 0o700); err != nil {
			log.Logger.Errorf("mkdir for session key fail: %v", err)
			os.Exit(1)
		}
		if err := os.WriteFile(sessionKeyFile, buf, 0o600); err != nil {
			log.Logger.Errorf("write session key fail: %v", err)
			os.Exit(1)
		}
		sessionSecret = buf
	})
	return sessionSecret
}
