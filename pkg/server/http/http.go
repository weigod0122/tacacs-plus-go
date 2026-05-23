package http

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"tacacs/pkg/public/log"
	"time"

	"tacacs/pkg/public/utils"
	"tacacs/pkg/server/internal/slog"

	"github.com/gin-gonic/gin"
)

var (
	srv                     = &http.Server{}
	addrPort                string
	ignoreHttpApiLogUriList []string
)

func Start(AddrPort string) {
	gin.SetMode(gin.ReleaseMode)

	addrPort = AddrPort

	app := gin.New()

	app.Use(bodyLimitMiddleware(), httpApiLog(), gin.Recovery())

	configRoutes(app)

	srv = &http.Server{
		Addr:    AddrPort,
		Handler: app,
	}
	go func() {
		// listener 失败时直接退出进程,而不是 silent log。
		// 否则 main 仍卡在 sigwait,deploy.sh 12s 观察期看到 PID 还在会误判"起来了",
		// 实际对外根本没有服务(典型场景:8899 被孤儿进程占着 bind failed)。
		// http.ErrServerClosed 是 Shutdown 触发的正常退出,不是错误。
		err := srv.ListenAndServe()
		if err == nil || err == http.ErrServerClosed {
			return
		}
		log.Logger.Errorf("http listening: %s failed, exiting: %s", AddrPort, err)
		os.Exit(1)
	}()
	log.Logger.Infof("http listening: %s", AddrPort)

	go updatePasswordErrUserUpdate()
	go checkPasswordErrUserUpdate()
}

func Stop() {
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Logger.Errorf("http listening: %s is error, because %s", addrPort, err)
	}
	log.Logger.Infof("http stop listening: %s", addrPort)
}

func httpApiLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 开始时间
		startTime := time.Now()

		//请求body
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			if isBodyTooLargeErr(err) {
				respondBodyTooLarge(c)
			} else {
				c.JSON(http.StatusBadRequest, gin.H{"error": "can't read body"})
			}
			goto end
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// 处理请求
		c.Next()

	end:

		// 结束时间
		endTime := time.Now()

		// 执行时间
		latencyTime := endTime.Sub(startTime)

		// 请求方式
		reqMethod := c.Request.Method

		// 请求路由
		reqUri := c.Request.URL.RequestURI()
		if utils.IsValueInList(reqUri, ignoreHttpApiLogUriList) {
			return
		}

		// 状态码
		statusCode := c.Writer.Status()

		// 请求IP
		clientIP := c.ClientIP()

		// 日志格式
		slog.HttpApiLogger.Infof("|code: %3d |latencyTime: %13v |srcIp: %15s |method: %8s |uri: %-100s |body: %v",
			statusCode,
			latencyTime,
			clientIP,
			reqMethod,
			reqUri,
			strings.ReplaceAll(string(bodyBytes), "\n", ""),
		)

	}
}

func ignoreHttpApiLogUriListAdd(uri string) {
	ignoreHttpApiLogUriList = append(ignoreHttpApiLogUriList, uri)
}
