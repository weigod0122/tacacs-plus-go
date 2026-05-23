package http

import (
	"context"
	"crypto/tls"
	"embed"
	"errors"
	"net/http"
	"os"
	"time"

	"tacacs/pkg/public/cfg"
	"tacacs/pkg/public/log"

	"github.com/gin-gonic/gin"
)

var (
	srv      = &http.Server{}
	addrPort string
)

// Start 启动 swm 的 HTTP/HTTPS 监听。
//
// 证书行为:
//   - cfg.cert_file 和 cfg.key_file 双空 → 明文 HTTP (ListenAndServe)。
//     swm 设置的 cookie 写死了 Secure 标记 (pkg/swm/http/{csrf,route,handler}.go),
//     而浏览器只在 secure context 下才接受/回传 Secure cookie, 所以 HTTP 模式只适合
//     两种场景:
//     1. 前面挂 HTTPS 反代 (nginx/ALB 终结 TLS), swm 跑在反代后的内网 HTTP;
//     浏览器侧看到的是 HTTPS, Secure cookie 正常工作。
//     2. 仅本机自连开发, 即浏览器地址栏用 http://localhost / http://127.0.0.1
//     (主流浏览器把 localhost / 回环地址视为 secure context, 是个明文 HTTP 的特例)。
//     用 LAN IP / 域名 (含 *.local) 直连 HTTP 都不在 secure context 内, Set-Cookie
//     会被浏览器丢弃, 登录走不通——这种场景必须走方案 1。
//   - 双填 → HTTPS (ListenAndServeTLS 直接从磁盘读 cert/key)。
//   - 半填 → 配置错误,直接 os.Exit(1),不让"半 TLS"状态混过去。
//
// staticFS 是编译期嵌入的前端资源 (pkg/swm/static/),由 route 转给 gin。
func Start(AddrPort string, staticFS embed.FS) {
	c := cfg.SwmConfig()
	certPath, keyPath := c.CertFile, c.KeyFile

	if (certPath == "") != (keyPath == "") {
		log.Logger.Errorf("cfg.cert_file 和 cfg.key_file 必须同时配置或同时留空, 当前 cert=%q key=%q", certPath, keyPath)
		os.Exit(1)
	}

	gin.SetMode(gin.ReleaseMode)
	app := gin.New()
	app.Use(gin.Recovery())

	route(app, staticFS)

	srv = &http.Server{
		Addr:              AddrPort,
		Handler:           app,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	addrPort = AddrPort

	if certPath == "" {
		log.Logger.Warningf("swm 以明文 HTTP 运行 (cert_file/key_file 留空); 仅适合: ①前面挂 HTTPS 反代终结 TLS, ②浏览器用 http://localhost 或 127.0.0.1 自连开发. 用 LAN IP/域名直连会因 Secure cookie 不在 secure context 而登录失败")
		go func() {
			// listener 失败必须让进程退出,否则 deploy.sh 12s 观察期检测不到 8897 没起来。
			// http.ErrServerClosed 是 Shutdown 触发的正常退出。
			err := srv.ListenAndServe()
			if err == nil || errors.Is(err, http.ErrServerClosed) {
				return
			}
			log.Logger.Errorf("http listening: %s failed, exiting: %s", AddrPort, err)
			os.Exit(1)
		}()
		log.Logger.Infof("http listening: %s (no TLS)", AddrPort)
		return
	}

	srv.TLSConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		},
		PreferServerCipherSuites: true,
	}

	go func() {
		// ListenAndServeTLS 内部会用 srv.TLSConfig 并把 certPath/keyPath 加载进 Certificates。
		// listener 失败让进程退出(理由同 HTTP 分支)。
		err := srv.ListenAndServeTLS(certPath, keyPath)
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return
		}
		log.Logger.Errorf("https listening: %s failed, exiting: %s", AddrPort, err)
		os.Exit(1)
	}()
	log.Logger.Infof("https listening: %s (cert: %s)", AddrPort, certPath)
}

func Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Logger.Errorf("http shutdown: %s is error, because %s", addrPort, err)
	}
	log.Logger.Infof("http stop listening: %s", addrPort)
}
