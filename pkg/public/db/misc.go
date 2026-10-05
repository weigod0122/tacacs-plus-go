package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/logquery"
)

// tacacs_misc 中存放操作日志展示配置：外部日志系统跳转的三个 URL、三个可见性开关，
// 以及展示方式、ClickHouse 连接和三类字段映射。
// 三个 URL 分别对应 TACACS+ 的三种协议日志(认证 / 授权 / 记账),
// 让管理员把不同维度的查询投到不同的外部日志系统(或同一系统的不同视图)。
// 三个 Visible* 字段独立控制对应类型按钮是否对普通用户可见,
// 默认 "0" 仅管理员可见;任意一个为 "1" 时普通用户侧栏即出现「操作日志」入口,
// 进入后只能看到 visible="1" 且 url 非空的那几个按钮。
//
// 注:tacacs_misc 表还有一列 description,描述每个 key 的用途,仅给 DBA 看;
// 权威源是下面的 MiscDescriptions 这张表,SyncMiscDescriptions 在 server 启动
// 时把代码里的值同步到 DB(行不存在 → 连同 v=” 一起 INSERT;行存在但
// description 跟代码不一致 → 只 UPDATE description,绝不动 v)。
// 业务运行期的 UpsertMisc 只写 (k, v),从不动 description。
// 新增 key 时:在常量、MiscDescriptions、和 handler 里各加一行即可,SQL 不动。
const (
	MiscKeyLogDisplayMode             = "log_display_mode"
	MiscKeyLogClickHouseConfig        = "log_clickhouse_config"
	MiscKeyLogClickHouseMappingAuth   = "log_clickhouse_mapping_authen"
	MiscKeyLogClickHouseMappingAuthor = "log_clickhouse_mapping_author"
	MiscKeyLogClickHouseMappingAcct   = "log_clickhouse_mapping_account"

	MiscKeyLogRedirectURLAuthen      = "log_redirect_url_authen"
	MiscKeyLogRedirectURLAuthor      = "log_redirect_url_author"
	MiscKeyLogRedirectURLAccount     = "log_redirect_url_account"
	MiscKeyLogRedirectVisibleAuthen  = "log_redirect_visible_authen"
	MiscKeyLogRedirectVisibleAuthor  = "log_redirect_visible_author"
	MiscKeyLogRedirectVisibleAccount = "log_redirect_visible_account"
)

// MiscDescriptions 是 tacacs_misc 所有 key 的用途说明,权威源在代码侧。
// server 启动调 SyncMiscDescriptions 把这里每条文本写入/校正 DB 的 description
// 列;新增 key 时:加常量 + 这个 map 各一行,DBA 排障时直接 SELECT 即懂。
var MiscDescriptions = map[string]string{
	MiscKeyLogDisplayMode:             "操作日志展示方式:external=外部链接,clickhouse=ClickHouse直查",
	MiscKeyLogClickHouseConfig:        "操作日志 ClickHouse 连接配置(JSON);密码仅由 server 使用,不回传前端",
	MiscKeyLogClickHouseMappingAuth:   "操作日志 ClickHouse 认证表与 AuthenInfo 字段映射(JSON)",
	MiscKeyLogClickHouseMappingAuthor: "操作日志 ClickHouse 授权表与 AuthorInfo 字段映射(JSON)",
	MiscKeyLogClickHouseMappingAcct:   "操作日志 ClickHouse 记账表与 AccountInfo 字段映射(JSON)",
	MiscKeyLogRedirectURLAuthen:       "外部日志系统-认证日志跳转 URL;空则前端「操作日志」页该按钮 disable;非空必须 http(s)://",
	MiscKeyLogRedirectURLAuthor:       "外部日志系统-授权日志跳转 URL;空则前端「操作日志」页该按钮 disable;非空必须 http(s)://",
	MiscKeyLogRedirectURLAccount:      "外部日志系统-记账日志跳转 URL;空则前端「操作日志」页该按钮 disable;非空必须 http(s)://",
	MiscKeyLogRedirectVisibleAuthen:   `认证日志按钮是否对普通用户可见:"1"=可见,"0"=仅管理员(默认)`,
	MiscKeyLogRedirectVisibleAuthor:   `授权日志按钮是否对普通用户可见:"1"=可见,"0"=仅管理员(默认)`,
	MiscKeyLogRedirectVisibleAccount:  `记账日志按钮是否对普通用户可见:"1"=可见,"0"=仅管理员(默认)`,
}

// ClickHouseLogConfig is persisted in tacacs_misc as a compact JSON value.
// The password is deliberately never included in the public HTTP response;
// this type stays in the server-side persistence layer.
type ClickHouseLogConfig struct {
	Address  string                                `json:"address"`
	Username string                                `json:"username"`
	Password string                                `json:"password"`
	Database string                                `json:"database"`
	Mappings map[logquery.LogType]logquery.Mapping `json:"mappings"`
}

func GetLogDisplayMode() (string, error) {
	v, err := GetMisc(MiscKeyLogDisplayMode)
	if err != nil {
		return "", err
	}
	if v == "clickhouse" {
		return "clickhouse", nil
	}
	return "external", nil
}

func GetClickHouseLogConfig() (ClickHouseLogConfig, error) {
	var cfg ClickHouseLogConfig
	raw, err := GetMisc(MiscKeyLogClickHouseConfig)
	if err != nil {
		return cfg, err
	}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return cfg, fmt.Errorf("decode clickhouse log config: %w", err)
		}
	}
	if cfg.Mappings == nil {
		cfg.Mappings = make(map[logquery.LogType]logquery.Mapping)
	}
	for typ, key := range map[logquery.LogType]string{
		logquery.LogTypeAuthen:  MiscKeyLogClickHouseMappingAuth,
		logquery.LogTypeAuthor:  MiscKeyLogClickHouseMappingAuthor,
		logquery.LogTypeAccount: MiscKeyLogClickHouseMappingAcct,
	} {
		raw, err := GetMisc(key)
		if err != nil {
			return cfg, err
		}
		if raw == "" {
			continue
		}
		var mapping logquery.Mapping
		if err := json.Unmarshal([]byte(raw), &mapping); err != nil {
			return cfg, fmt.Errorf("decode clickhouse %s mapping: %w", typ, err)
		}
		cfg.Mappings[typ] = mapping
	}
	return cfg, nil
}

func SaveClickHouseLogConfig(cfg ClickHouseLogConfig) error {
	if cfg.Mappings == nil {
		cfg.Mappings = make(map[logquery.LogType]logquery.Mapping)
	}
	// Keep connection credentials in one JSON document and mappings in their
	// own values so each value remains comfortably below tacacs_misc.v's limit.
	connectionJSON, err := json.Marshal(struct {
		Address  string `json:"address"`
		Username string `json:"username"`
		Password string `json:"password"`
		Database string `json:"database"`
	}{cfg.Address, cfg.Username, cfg.Password, cfg.Database})
	if err != nil {
		return err
	}
	values := map[string]string{
		MiscKeyLogClickHouseConfig: string(connectionJSON),
	}
	for typ, key := range map[logquery.LogType]string{
		logquery.LogTypeAuthen:  MiscKeyLogClickHouseMappingAuth,
		logquery.LogTypeAuthor:  MiscKeyLogClickHouseMappingAuthor,
		logquery.LogTypeAccount: MiscKeyLogClickHouseMappingAcct,
	} {
		mapping := cfg.Mappings[typ]
		b, err := json.Marshal(mapping)
		if err != nil {
			return fmt.Errorf("encode clickhouse %s mapping: %w", typ, err)
		}
		values[key] = string(b)
	}
	values[MiscKeyLogDisplayMode] = "clickhouse"
	return UpsertMiscBatch(values)
}

func SaveLogDisplayMode(mode string) error {
	if mode != "external" && mode != "clickhouse" {
		return fmt.Errorf("unsupported log display mode %q", mode)
	}
	return UpsertMisc(MiscKeyLogDisplayMode, mode)
}

// SyncMiscDescriptions 启动时调一次,把 MiscDescriptions 里每个 key 的 description
// 写入/校正到 DB。语义:
//   - key 不存在:INSERT (k, v=”, description) —— 同时把"空 value"行也建好,
//     避免首次 GetMisc 仍返回 ErrNoRows 走空串分支(行为一致,但 DBA SELECT
//     时能看到完整的配置 key 列表)。
//   - key 已存在:ON DUPLICATE 分支只 UPDATE description,**不**带 VALUES(v),
//     否则会把已配置的 URL / 可见性值覆盖回空串。
//
// client 进程 DbWrite==nil 直接 noop(与 BumpMetaVersion / UpsertMisc 同源策略)。
// 任何单 key 同步失败立即返回错误,让 server 启动失败而非半成品上线。
func SyncMiscDescriptions() error {
	if DbWrite == nil {
		return nil
	}
	for key, desc := range MiscDescriptions {
		_, err := DbWrite.Exec(
			"INSERT INTO tacacs_misc (k, v, description) VALUES (?, '', ?) "+
				"ON DUPLICATE KEY UPDATE description = VALUES(description)",
			key, desc,
		)
		if err != nil {
			log.Logger.Errorf("sync tacacs_misc description key=%s failed: %v", key, err)
			return err
		}
	}
	return nil
}

// GetMisc 读取 tacacs_misc 中指定 key 的 value。
// 不存在时返回空串 + nil（让调用方按 empty state 处理，不是错误）。
func GetMisc(key string) (string, error) {
	var v string
	err := DbRead.QueryRow("SELECT v FROM tacacs_misc WHERE k = ?", key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		log.Logger.Errorf("get tacacs_misc key=%s failed: %v", key, err)
		return "", err
	}
	return v, nil
}

// UpsertMisc 写入/更新 tacacs_misc 中指定 key 的 value。
// 走 DbWrite；client 进程 DbWrite==nil 时直接 noop（防误调，与 BumpMetaVersion 同源策略）。
// 注意：故意只 UPDATE v,绝不动 description —— description 的权威源是
// MiscDescriptions,由 SyncMiscDescriptions 在启动时维护。
func UpsertMisc(key, value string) error {
	if DbWrite == nil {
		return nil
	}
	_, err := DbWrite.Exec(
		"INSERT INTO tacacs_misc (k, v) VALUES (?, ?) ON DUPLICATE KEY UPDATE v = VALUES(v)",
		key, value,
	)
	if err != nil {
		log.Logger.Errorf("upsert tacacs_misc key=%s failed: %v", key, err)
		return err
	}
	return nil
}

// UpsertMiscBatch persists a logically single settings change atomically.
func UpsertMiscBatch(values map[string]string) error {
	if DbWrite == nil {
		return nil
	}
	tx, err := DbWrite.Begin()
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()
	for key, value := range values {
		if _, err := tx.Exec(
			"INSERT INTO tacacs_misc (k, v) VALUES (?, ?) ON DUPLICATE KEY UPDATE v = VALUES(v)",
			key, value,
		); err != nil {
			return fmt.Errorf("upsert misc key=%s: %w", key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false
	return nil
}
