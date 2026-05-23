package feishu

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"tacacs/pkg/public/cfg"
	"tacacs/pkg/public/db"
	"time"

	"github.com/bytedance/sonic"
)

// appCreds 解析飞书凭据：依次检查 ServerConfig → ClientConfig → SwmConfig，
// 取第一个非空的 AppId+AppSecret。三者皆 nil 时返回空串——上层 notifyEnabled()
// 会据此短路所有发送路径。
func appCreds() (string, string) {
	if c := cfg.ServerConfig(); c != nil {
		id := c.Feishu.ResolveAppId()
		secret := c.Feishu.ResolveAppSecret()
		if id != "" && secret != "" {
			return id, secret
		}
	}
	if c := cfg.ClientConfig(); c != nil {
		id := c.Feishu.ResolveAppId()
		secret := c.Feishu.ResolveAppSecret()
		if id != "" && secret != "" {
			return id, secret
		}
	}
	if c := cfg.SwmConfig(); c != nil {
		id := c.Feishu.ResolveAppId()
		secret := c.Feishu.ResolveAppSecret()
		if id != "" && secret != "" {
			return id, secret
		}
	}
	return "", ""
}

// AppCreds 是 appCreds 的公开导出，给 feishuws 长连接客户端使用，
// 保持和 token 缓存路径同源（cfg → 环境变量 → 历史硬编码）。
func AppCreds() (string, string) {
	return appCreds()
}

// 飞书 tenant_access_token 缓存。提前 5 分钟刷新，避免飞书频控（一分钟若干次）。
type tokenCache struct {
	mu       sync.RWMutex
	token    string
	expireAt time.Time
}

var globalTokenCache tokenCache

const tokenRefreshAhead = 5 * time.Minute

// getCachedTenantAccessToken 是包内统一的 token 获取入口，所有飞书 API 调用都该走它。
func getCachedTenantAccessToken() (string, error) {
	globalTokenCache.mu.RLock()
	if globalTokenCache.token != "" && time.Until(globalTokenCache.expireAt) > tokenRefreshAhead {
		token := globalTokenCache.token
		globalTokenCache.mu.RUnlock()
		return token, nil
	}
	globalTokenCache.mu.RUnlock()

	globalTokenCache.mu.Lock()
	defer globalTokenCache.mu.Unlock()
	// double-check：另一个 goroutine 可能已经刷新了
	if globalTokenCache.token != "" && time.Until(globalTokenCache.expireAt) > tokenRefreshAhead {
		return globalTokenCache.token, nil
	}

	id, secret := appCreds()
	token, expireSec, err := fetchTenantAccessToken(id, secret)
	if err != nil {
		return "", err
	}
	globalTokenCache.token = token
	globalTokenCache.expireAt = time.Now().Add(time.Duration(expireSec) * time.Second)
	return token, nil
}

// fetchTenantAccessToken 直连飞书 API 拿一次 token。仅 getCachedTenantAccessToken
// 调用，外部不应直接用——否则等于绕过缓存。
func fetchTenantAccessToken(appId, appSecret string) (string, int, error) {
	url := "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal"

	payload := []byte(fmt.Sprintf(`{
		"app_id": "%v",
		"app_secret": "%v"
	}`, appId, appSecret))

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("http code is %v, body is %v", resp.Status, string(body))
	}

	type response struct {
		Code              int    `json:"code"`
		Expire            int    `json:"expire"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}

	result := &response{}
	err = sonic.Unmarshal(body, result)
	if err != nil {
		return "", 0, fmt.Errorf("unmarshal err: %v, body: %v", err, string(body))
	}

	if result.Msg != "ok" || result.Code != 0 {
		return "", 0, fmt.Errorf("http code is %v, body is %v", result.Code, string(body))
	}
	return result.TenantAccessToken, result.Expire, nil
}

func GetUserIdByBasicInfo(email, phoneNumber string) (string, error) {
	return getIdByBasicInfo(email, phoneNumber, "user_id")
}

// GetOpenIdByBasicInfo 走同一个 batch_get_id 接口拿 open_id 而不是 user_id。
// 给 ws 卡片回调反向匹配 admin 用——回调天然给 OpenID，比拿 email/mobile 再
// 反查少一层接口，也避免企业字段级权限缺失把 email/mobile 剥成空串。
func GetOpenIdByBasicInfo(email, phoneNumber string) (string, error) {
	return getIdByBasicInfo(email, phoneNumber, "open_id")
}

func getIdByBasicInfo(email, phoneNumber, idType string) (string, error) {
	feishuToken, err := getCachedTenantAccessToken()
	if err != nil {
		return "", fmt.Errorf("get tenant access token err: %v", err)
	}

	url := fmt.Sprintf("https://open.feishu.cn/open-apis/contact/v3/users/batch_get_id?user_id_type=%s", idType)

	type request struct {
		Emails          []string `json:"emails,omitempty"`
		Mobiles         []string `json:"mobiles,omitempty"`
		IncludeResigned bool     `json:"include_resigned"`
	}
	reqBody := request{IncludeResigned: true}
	if email != "" {
		reqBody.Emails = []string{email}
	}
	if phoneNumber != "" {
		reqBody.Mobiles = []string{phoneNumber}
	}

	payload, err := sonic.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+feishuToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http code is %v, body is %v", resp.Status, string(body))
	}

	type userInfo struct {
		Email  string `json:"email"`
		Mobile string `json:"mobile"`
		UserId string `json:"user_id"`
	}
	type response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			UserList []userInfo `json:"user_list"`
		} `json:"data"`
	}

	result := &response{}
	if err = sonic.Unmarshal(body, result); err != nil {
		return "", fmt.Errorf("unmarshal err: %v, body: %v", err, string(body))
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu api err, code: %v, msg: %v, body: %v", result.Code, result.Msg, string(body))
	}

	var emailUserId, mobileUserId string
	for _, u := range result.Data.UserList {
		if u.Email != "" {
			emailUserId = u.UserId
		}
		if u.Mobile != "" {
			mobileUserId = u.UserId
		}
	}

	if emailUserId == "" && mobileUserId == "" {
		return "", fmt.Errorf("user not found by email(%v) or phone(%v), body: %v", email, phoneNumber, string(body))
	}
	if emailUserId != "" && mobileUserId != "" && emailUserId != mobileUserId {
		return "", fmt.Errorf("email(%v) -> id(%v) 与 phone(%v) -> id(%v) 不一致", email, emailUserId, phoneNumber, mobileUserId)
	}
	if emailUserId != "" {
		return emailUserId, nil
	}
	return mobileUserId, nil
}

// sendMessage 给 user_id 发送一条消息（msg_type 任意，content 已序列化为 string）。
// 返回 message_id 供后续加急等操作使用。
func sendMessage(userId, msgType, content string) (string, error) {
	feishuToken, err := getCachedTenantAccessToken()
	if err != nil {
		return "", fmt.Errorf("get tenant access token err: %v", err)
	}

	url := "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=user_id"

	type request struct {
		ReceiveId string `json:"receive_id"`
		MsgType   string `json:"msg_type"`
		Content   string `json:"content"`
	}
	payload, err := sonic.Marshal(request{
		ReceiveId: userId,
		MsgType:   msgType,
		Content:   content,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+feishuToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http code is %v, body is %v", resp.Status, string(body))
	}

	type response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageId string `json:"message_id"`
		} `json:"data"`
	}
	result := &response{}
	if err = sonic.Unmarshal(body, result); err != nil {
		return "", fmt.Errorf("unmarshal err: %v, body: %v", err, string(body))
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu api err, code: %v, msg: %v, body: %v", result.Code, result.Msg, string(body))
	}
	return result.Data.MessageId, nil
}

type UrgentType string

const (
	UrgentApp   UrgentType = "urgent_app"
	UrgentSMS   UrgentType = "urgent_sms"
	UrgentPhone UrgentType = "urgent_phone"
)

func urgentMessage(messageId string, userIds []string, urgentType UrgentType) error {
	feishuToken, err := getCachedTenantAccessToken()
	if err != nil {
		return fmt.Errorf("get tenant access token err: %v", err)
	}

	url := fmt.Sprintf("https://open.feishu.cn/open-apis/im/v1/messages/%s/%s?user_id_type=user_id", messageId, urgentType)

	type request struct {
		UserIdList []string `json:"user_id_list"`
	}
	payload, err := sonic.Marshal(request{UserIdList: userIds})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+feishuToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http code is %v, body is %v", resp.Status, string(body))
	}

	type response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	result := &response{}
	if err = sonic.Unmarshal(body, result); err != nil {
		return fmt.Errorf("unmarshal err: %v, body: %v", err, string(body))
	}
	if result.Code != 0 {
		return fmt.Errorf("feishu urgent api err, code: %v, msg: %v, body: %v", result.Code, result.Msg, string(body))
	}
	return nil
}

func sendByTacacsUserName(userName []string, send func(userId string) error) error {
	errMap := make(map[string]string)
	for _, u := range userName {
		tacacsUserInfo, err := db.GetTacacsUserInfoByUserName(u)
		if err != nil {
			return fmt.Errorf("get tacacs user info err: %v", err)
		}
		if tacacsUserInfo == nil {
			return fmt.Errorf("userName(%v)`s tacacs user info is nil", userName)
		}
		userId, err := GetUserIdByBasicInfo(tacacsUserInfo.Email, tacacsUserInfo.PhoneNumber)
		if err != nil {
			return fmt.Errorf("get feishu user id err: %v", err)
		}
		if err := send(userId); err != nil {
			errMap[userId] = err.Error()
		}
	}
	if len(errMap) > 0 {
		err := ""
		for userId, msg := range errMap {
			err += fmt.Sprintf("send to user(id:%v) failed, err: %v;", userId, msg)
		}
		return errors.New(strings.TrimRight(err, ";"))
	}
	return nil
}
