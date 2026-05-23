// Package feishuws 把飞书长连接（WebSocket）客户端作为 tacacs-server 的一个
// 子系统启停。订阅卡片按钮事件 OnP2CardActionTrigger——admin 在飞书内直接点
// 「通过 / 驳回」即可完成审批，无需切到 SwM 网页。
//
// 内网部署不开放公网回调 URL，所以选 larkws 长连接：客户端主动握手到飞书云，
// 双向消息走 WebSocket，断网由 SDK 自动指数退避重连。
package feishuws

import (
	"context"
	"sync"
	"tacacs/pkg/public/cfg"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/notify/feishu"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

type Service struct {
	mu      sync.Mutex
	cli     *larkws.Client
	cancel  context.CancelFunc
	started bool
}

// New 按当前 cfg.Feishu 构造一个 Service。enabled=false 或凭据空时
// Start 是 no-op——开发环境不会向飞书发起任何连接。
func New() *Service {
	return &Service{}
}

// Start 起 goroutine 跑 cli.Start(ctx)。已启动时直接返回 nil（幂等）。
func (s *Service) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return nil
	}

	c := cfg.ServerConfig()
	if c == nil || !c.Feishu.Enabled {
		log.Logger.Info("feishu ws skipped: cfg.Feishu.Enabled=false")
		return nil
	}
	appId, appSecret := feishu.AppCreds()
	if appId == "" || appSecret == "" {
		log.Logger.Info("feishu ws skipped: app_id/app_secret empty")
		return nil
	}

	handler := dispatcher.NewEventDispatcher("", "").
		OnP2CardActionTrigger(handleCardAction)

	s.cli = larkws.NewClient(
		appId, appSecret,
		larkws.WithEventHandler(handler),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.started = true

	go func() {
		log.Logger.Info("feishu ws starting…")
		if err := s.cli.Start(ctx); err != nil && ctx.Err() == nil {
			log.Logger.Errorf("feishu ws stopped with err: %v", err)
		}
		log.Logger.Info("feishu ws stopped")
	}()
	return nil
}

// Stop 取消 ctx，让 cli.Start 退出。waitGroup 在 catchStopSignalServer 那侧 Wait。
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.started = false
}
