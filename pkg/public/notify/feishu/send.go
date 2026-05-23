package feishu

import (
	"fmt"
	"tacacs/pkg/public/cfg"

	"github.com/bytedance/sonic"
)

// notifyEnabled 在 cfg.Feishu.Enabled 显式为 false 或凭据缺失时返回 false，
// 让所有对外发送路径短路。
//   - server 进程：依 cfg.ServerConfig().Feishu.Enabled 决定。
//   - client 进程：cfg.ServerConfig() 为 nil，仅靠 appCreds() 是否有凭据决定
//     （历史硬编码兜底始终给值），保持与文本路径同样的「能发就发」行为。
func notifyEnabled() bool {
	if c := cfg.ServerConfig(); c != nil && !c.Feishu.Enabled {
		return false
	}
	if c := cfg.ClientConfig(); c != nil && !c.Feishu.Enabled {
		return false
	}
	if c := cfg.SwmConfig(); c != nil && !c.Feishu.Enabled {
		return false
	}
	id, secret := appCreds()
	return id != "" && secret != ""
}

// SendCardToPersonByUserId 向飞书 user_id 发送一张 schema 2.0 交互卡片。
// card 是 cards.go 里 builder 返回的 map，本函数负责 JSON 序列化并交给
// sendMessage（msg_type=interactive）。
func SendCardToPersonByUserId(userId string, card map[string]any) error {
	if !notifyEnabled() {
		return nil
	}
	contentBytes, err := sonic.Marshal(card)
	if err != nil {
		return fmt.Errorf("marshal card err: %v", err)
	}
	_, err = sendMessage(userId, "interactive", string(contentBytes))
	return err
}

func SendUrgentCardToPersonByUserId(userId string, card map[string]any, urgentType UrgentType) error {
	if !notifyEnabled() {
		return nil
	}
	contentBytes, err := sonic.Marshal(card)
	if err != nil {
		return fmt.Errorf("marshal card err: %v", err)
	}
	msgId, err := sendMessage(userId, "interactive", string(contentBytes))
	if err != nil {
		return err
	}
	return urgentMessage(msgId, []string{userId}, urgentType)
}

// SendCardToPersonByTacacsUserName 按 tacacs 用户名批量发卡片。复用 sendByTacacsUserName
// 的查 user_id + 错误聚合逻辑——和文本路径完全对偶。
func SendCardToPersonByTacacsUserName(userName []string, card map[string]any) error {
	if !notifyEnabled() {
		return nil
	}
	return sendByTacacsUserName(userName, func(uid string) error {
		return SendCardToPersonByUserId(uid, card)
	})
}
