package http

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"tacacs/pkg/public/db"
	"tacacs/pkg/public/logquery"
	"tacacs/pkg/public/waitGroup"
	"tacacs/pkg/server/clickhouse"

	"github.com/gin-gonic/gin"
)

// logRedirectConfig 是外部日志系统跳转配置的对外 wire 形态。
// JSON 字段对齐前端：authen / author / account 三个 URL,以及它们各自
// 是否对普通用户可见的 visibleAuthen / visibleAuthor / visibleAccount 三个开关。
type logRedirectConfig struct {
	Mode           string `json:"mode,omitempty"`
	Authen         string `json:"authen"`
	Author         string `json:"author"`
	Account        string `json:"account"`
	VisibleAuthen  bool   `json:"visibleAuthen"`
	VisibleAuthor  bool   `json:"visibleAuthor"`
	VisibleAccount bool   `json:"visibleAccount"`
}

type clickHouseConfigRequest struct {
	Address  string                      `json:"address"`
	Username string                      `json:"username"`
	Password string                      `json:"password"`
	Database string                      `json:"database"`
	Mappings map[string]logquery.Mapping `json:"mappings"`
}

type systemLogConfigRequest struct {
	Mode       string                  `json:"mode"`
	External   logRedirectConfig       `json:"external"`
	ClickHouse clickHouseConfigRequest `json:"clickhouse"`
}

type clickHouseConfigView struct {
	Address            string                      `json:"address"`
	Username           string                      `json:"username"`
	Database           string                      `json:"database"`
	PasswordConfigured bool                        `json:"passwordConfigured"`
	Mappings           map[string]logquery.Mapping `json:"mappings"`
}

type systemLogConfigView struct {
	Mode       string               `json:"mode"`
	External   logRedirectConfig    `json:"external"`
	ClickHouse clickHouseConfigView `json:"clickhouse"`
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
	if mode, modeErr := db.GetLogDisplayMode(); modeErr == nil {
		cfg.Mode = mode
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
	if !requireAdminIdentity(c) {
		return
	}
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

	values := map[string]string{
		db.MiscKeyLogDisplayMode:            "external",
		db.MiscKeyLogRedirectURLAuthen:      authen,
		db.MiscKeyLogRedirectURLAuthor:      author,
		db.MiscKeyLogRedirectURLAccount:     account,
		db.MiscKeyLogRedirectVisibleAuthen:  boolToFlag(req.VisibleAuthen),
		db.MiscKeyLogRedirectVisibleAuthor:  boolToFlag(req.VisibleAuthor),
		db.MiscKeyLogRedirectVisibleAccount: boolToFlag(req.VisibleAccount),
	}
	if err := db.UpsertMiscBatch(values); err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("upsert log redirect config failed: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "log redirect config updated",
	})
}

// httpApiSystemGetLogConfig returns the full administrator settings view. The
// password itself is never returned; the UI uses passwordConfigured to decide
// whether an empty input means "keep the current password".
func httpApiSystemGetLogConfig(c *gin.Context) {
	view, err := loadSystemLogConfig()
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("get log config failed: %v", err),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "data": view})
}

func httpApiSystemSetLogConfig(c *gin.Context) {
	if !requireAdminIdentity(c) {
		return
	}
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()

	var req systemLogConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": fmt.Sprintf("invalid body: %v", err)})
		return
	}
	if req.Mode != "external" && req.Mode != "clickhouse" {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": "mode must be external or clickhouse"})
		return
	}
	if req.Mode == "external" {
		if err := validateRedirectConfig(req.External); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": err.Error()})
			return
		}
		if err := saveExternalLogConfig(req.External); err != nil {
			c.JSON(http.StatusFailedDependency, gin.H{"code": http.StatusFailedDependency, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "message": "log display mode updated"})
		return
	}

	ckCfg, err := resolveClickHouseConfig(req.ClickHouse)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": err.Error()})
		return
	}
	if err := clickhouse.DefaultManager.ValidateConfig(c.Request.Context(), ckCfg); err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{"code": http.StatusFailedDependency, "message": fmt.Sprintf("clickhouse validation failed: %v", err)})
		return
	}
	if err := db.SaveClickHouseLogConfig(ckCfg); err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{"code": http.StatusFailedDependency, "message": fmt.Sprintf("save clickhouse config failed: %v", err)})
		return
	}
	if err := clickhouse.DefaultManager.Apply(c.Request.Context(), ckCfg); err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{"code": http.StatusFailedDependency, "message": fmt.Sprintf("activate clickhouse config failed: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "message": "clickhouse log config updated"})
}

func httpApiSystemClickHouseTest(c *gin.Context) {
	if !requireAdminIdentity(c) {
		return
	}
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()

	var req clickHouseConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": fmt.Sprintf("invalid body: %v", err)})
		return
	}
	cfg, err := resolveClickHouseConfig(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": err.Error()})
		return
	}
	if err := clickhouse.DefaultManager.TestConnection(c.Request.Context(), cfg); err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{"code": http.StatusFailedDependency, "message": fmt.Sprintf("clickhouse connection failed: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "data": gin.H{"connected": true}})
}

func httpApiSystemClickHouseSchema(c *gin.Context) {
	if !requireAdminIdentity(c) {
		return
	}
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()

	var req clickHouseConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": fmt.Sprintf("invalid body: %v", err)})
		return
	}
	cfg, err := resolveClickHouseConfig(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": err.Error()})
		return
	}
	tables, err := clickhouse.DefaultManager.Discover(c.Request.Context(), cfg)
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{"code": http.StatusFailedDependency, "message": fmt.Sprintf("read clickhouse schema failed: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "data": gin.H{"tables": tables}})
}

func loadSystemLogConfig() (systemLogConfigView, error) {
	external, err := loadLogRedirectConfig()
	if err != nil {
		return systemLogConfigView{}, err
	}
	mode, err := db.GetLogDisplayMode()
	if err != nil {
		return systemLogConfigView{}, err
	}
	ck, err := db.GetClickHouseLogConfig()
	if err != nil {
		return systemLogConfigView{}, err
	}
	return systemLogConfigView{
		Mode:     mode,
		External: external,
		ClickHouse: clickHouseConfigView{
			Address:            ck.Address,
			Username:           ck.Username,
			Database:           ck.Database,
			PasswordConfigured: ck.Password != "",
			Mappings:           mappingView(ck.Mappings),
		},
	}, nil
}

func mappingView(src map[logquery.LogType]logquery.Mapping) map[string]logquery.Mapping {
	dst := make(map[string]logquery.Mapping, len(src))
	for typ, mapping := range src {
		dst[typ.String()] = mapping
	}
	return dst
}

func saveExternalLogConfig(req logRedirectConfig) error {
	authen, err := normalizeRedirectURL(req.Authen)
	if err != nil {
		return fmt.Errorf("authen %s", err)
	}
	author, err := normalizeRedirectURL(req.Author)
	if err != nil {
		return fmt.Errorf("author %s", err)
	}
	account, err := normalizeRedirectURL(req.Account)
	if err != nil {
		return fmt.Errorf("account %s", err)
	}
	return db.UpsertMiscBatch(map[string]string{
		db.MiscKeyLogDisplayMode:            "external",
		db.MiscKeyLogRedirectURLAuthen:      authen,
		db.MiscKeyLogRedirectURLAuthor:      author,
		db.MiscKeyLogRedirectURLAccount:     account,
		db.MiscKeyLogRedirectVisibleAuthen:  boolToFlag(req.VisibleAuthen),
		db.MiscKeyLogRedirectVisibleAuthor:  boolToFlag(req.VisibleAuthor),
		db.MiscKeyLogRedirectVisibleAccount: boolToFlag(req.VisibleAccount),
	})
}

func validateRedirectConfig(req logRedirectConfig) error {
	if _, err := normalizeRedirectURL(req.Authen); err != nil {
		return fmt.Errorf("authen %s", err)
	}
	if _, err := normalizeRedirectURL(req.Author); err != nil {
		return fmt.Errorf("author %s", err)
	}
	if _, err := normalizeRedirectURL(req.Account); err != nil {
		return fmt.Errorf("account %s", err)
	}
	return nil
}

func resolveClickHouseConfig(req clickHouseConfigRequest) (db.ClickHouseLogConfig, error) {
	stored, err := db.GetClickHouseLogConfig()
	if err != nil {
		return db.ClickHouseLogConfig{}, err
	}
	password := req.Password
	if strings.TrimSpace(password) == "" {
		password = stored.Password
	}
	if strings.TrimSpace(req.Address) == "" {
		req.Address = stored.Address
	}
	if strings.TrimSpace(req.Username) == "" {
		req.Username = stored.Username
	}
	if strings.TrimSpace(req.Database) == "" {
		req.Database = stored.Database
	}
	mappings := make(map[logquery.LogType]logquery.Mapping, len(req.Mappings))
	for rawType, mapping := range req.Mappings {
		typ, err := logquery.ParseLogType(rawType)
		if err != nil {
			return db.ClickHouseLogConfig{}, err
		}
		mappings[typ] = mapping
	}
	if len(mappings) == 0 {
		mappings = stored.Mappings
	}
	if strings.TrimSpace(req.Address) == "" || strings.TrimSpace(req.Database) == "" {
		return db.ClickHouseLogConfig{}, fmt.Errorf("clickhouse address and database are required")
	}
	return db.ClickHouseLogConfig{
		Address:  strings.TrimSpace(req.Address),
		Username: strings.TrimSpace(req.Username),
		Password: password,
		Database: strings.TrimSpace(req.Database),
		Mappings: mappings,
	}, nil
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
