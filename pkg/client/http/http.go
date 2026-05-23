package http

import (
	"context"
	"net/http"
	"os"
	"tacacs/pkg/public/log"

	"github.com/gin-gonic/gin"
)

var (
	srv      = &http.Server{}
	addrPort string
)

func Start(AddrPort string) {
	gin.SetMode(gin.ReleaseMode)

	addrPort = AddrPort

	app := gin.New()

	app.Use(qpsLimitMiddleware(), gin.Recovery())

	configRoutes(app)

	srv = &http.Server{
		Addr:    AddrPort,
		Handler: app,
	}
	go func() {
		// 同 server:listener 失败必须让进程退出,否则 deploy.sh 检测不到 8383 没起来。
		// http.ErrServerClosed 是 Shutdown 触发的正常退出。
		err := srv.ListenAndServe()
		if err == nil || err == http.ErrServerClosed {
			return
		}
		log.Logger.Errorf("http listening: %s failed, exiting: %s", AddrPort, err)
		os.Exit(1)
	}()
	log.Logger.Infof("http listening: %s", AddrPort)
}

func Stop() {
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Logger.Errorf("http listening: %s is error, because %s", addrPort, err)
	}
	log.Logger.Infof("http stop listening: %s", addrPort)
}
