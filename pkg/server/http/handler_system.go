package http

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"tacacs/pkg/public/db"
	"tacacs/pkg/public/waitGroup"

	"github.com/gin-gonic/gin"
)

// logRedirectConfig 是外部日志系统跳转配置的对外 wire 形态。
// JSON 字段对齐前端：authen / author / account 三个 URL,以及它们各自
// 是否对普通用户可见的 visibleAuthen / visibleAuthor / visibleAccount 三个开关。
type logRedirectConfig struct {
	Authen         string `json:"authen"`
	Author         string `json:"author"`
	Account        string `json:"account"`
	VisibleAuthen  bool   `json:"visibleAuthen"`
	VisibleAuthor  bool   `json:"visibleAuthor"`
	VisibleAccount bool   `json:"visibleAccount"`
}

// httpApiSystemGetLogRedirectConfig 返回当前配置的三个跳转 URL + 三个可见性开关。
// 对所有已登录用户开放（method=GET 不在 adminWritePrefixes 内）,
// 这样普通用户登录时前端能据此决定是否渲染「操作日志」入口。
func httpApiSystemGetLogRedirectConfig(c *gin.Context) {
	cfg, err := loadLogRedirectConfig()
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("get log redirect config failed: %v", err),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": http.StatusOK,
		"data": cfg,
	})
}

// httpApiSystemSetLogRedirectConfig 一次性写入三个 URL + 三个可见性开关共 6 个 key。
// 仅管理员（POST 命中 adminWritePrefixes:/tacacs/system/）。
// 三个 URL 都允许空串(视为该协议未配置,前端按钮 disable);非空时必须是 http(s):// 绝对地址。
// 三个 Visible* 独立落库为 "1" / "0",对应类型按钮是否对普通用户可见。
func httpApiSystemSetLogRedirectConfig(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()

	var req logRedirectConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": fmt.Sprintf("invalid body: %v", err),
		})
		return
	}

	authen, err := normalizeRedirectURL(req.Authen)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": "authen " + err.Error()})
		return
	}
	author, err := normalizeRedirectURL(req.Author)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": "author " + err.Error()})
		return
	}
	account, err := normalizeRedirectURL(req.Account)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": "account " + err.Error()})
		return
	}

	for _, kv := range []struct{ k, v string }{
		{db.MiscKeyLogRedirectURLAuthen, authen},
		{db.MiscKeyLogRedirectURLAuthor, author},
		{db.MiscKeyLogRedirectURLAccount, account},
		{db.MiscKeyLogRedirectVisibleAuthen, boolToFlag(req.VisibleAuthen)},
		{db.MiscKeyLogRedirectVisibleAuthor, boolToFlag(req.VisibleAuthor)},
		{db.MiscKeyLogRedirectVisibleAccount, boolToFlag(req.VisibleAccount)},
	} {
		if err := db.UpsertMisc(kv.k, kv.v); err != nil {
			c.JSON(http.StatusFailedDependency, gin.H{
				"code":    http.StatusFailedDependency,
				"message": fmt.Sprintf("upsert misc key=%s failed: %v", kv.k, err),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "log redirect config updated",
	})
}

func loadLogRedirectConfig() (logRedirectConfig, error) {
	var cfg logRedirectConfig
	for _, kv := range []struct {
		key string
		dst *string
	}{
		{db.MiscKeyLogRedirectURLAuthen, &cfg.Authen},
		{db.MiscKeyLogRedirectURLAuthor, &cfg.Author},
		{db.MiscKeyLogRedirectURLAccount, &cfg.Account},
	} {
		v, err := db.GetMisc(kv.key)
		if err != nil {
			return cfg, err
		}
		*kv.dst = v
	}
	for _, kv := range []struct {
		key string
		dst *bool
	}{
		{db.MiscKeyLogRedirectVisibleAuthen, &cfg.VisibleAuthen},
		{db.MiscKeyLogRedirectVisibleAuthor, &cfg.VisibleAuthor},
		{db.MiscKeyLogRedirectVisibleAccount, &cfg.VisibleAccount},
	} {
		v, err := db.GetMisc(kv.key)
		if err != nil {
			return cfg, err
		}
		*kv.dst = v == "1"
	}
	return cfg, nil
}

func boolToFlag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// normalizeRedirectURL 校验单条跳转 URL：
//   - 空串合法（视为该协议未配置）
//   - 非空必须是 http(s):// 绝对地址
//   - 返回前后空白都已 trim 的规范化串
func normalizeRedirectURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("url 必须是合法的 http(s):// 绝对地址")
	}
	return raw, nil
}
