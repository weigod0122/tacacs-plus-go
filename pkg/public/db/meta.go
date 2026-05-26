package db

import (
	"tacacs/pkg/public/cfg"
	"tacacs/pkg/public/log"
)

// tacacs_meta 的 6 个固定 key。顺序必须与 GetTablesUpdateTime 拼接顺序一致。
const (
	MetaKeyUser     = "user"
	MetaKeyRole     = "role"
	MetaKeyServer   = "server"
	MetaKeyCommand  = "command"
	MetaKeyOnDuty   = "on_duty"
	MetaKeyApproval = "approval"
)

// MetaKeys 是 6 个 key 的固定顺序，供 GetTablesUpdateTime 拼接与
// RefreshAllMeta 全量 bump 使用。
var MetaKeys = []string{
	MetaKeyUser, MetaKeyRole, MetaKeyServer,
	MetaKeyCommand, MetaKeyOnDuty, MetaKeyApproval,
}

// BumpMetaVersion 给指定 key 的版本号 +1，触发 client 缓存失效。
//
// 仅在 cfg.ServerConfig().DatabaseTriggers == false 时实际写库；
// 启用了 DB 触发器的环境下应用层不再重复写，避免与 trigger 双写。
//
// 失败仅打日志：bump 丢失不会引发数据不一致，最坏情况 client 5min 兜底刷新。
func BumpMetaVersion(key string) {
	if conf := cfg.ServerConfig(); conf == nil || conf.DatabaseTriggers {
		return
	}
	if DbWrite == nil {
		return
	}
	_, err := DbWrite.Exec(
		"INSERT INTO tacacs_meta (k, version) VALUES (?, 1) ON DUPLICATE KEY UPDATE version = version + 1",
		key,
	)
	if err != nil {
		log.Logger.Errorf("bump tacacs_meta key=%s failed: %v", key, err)
	}
}

// RefreshAllMeta 把 6 个 key 全部 +1，无视当前 DatabaseTriggers 配置。
// 给管理员手动触发缓存失效用，不管被监控表是否真有变更都强制刷新。
func RefreshAllMeta() error {
	if DbWrite == nil {
		return nil
	}
	for _, k := range MetaKeys {
		_, err := DbWrite.Exec(
			"INSERT INTO tacacs_meta (k, version) VALUES (?, 1) ON DUPLICATE KEY UPDATE version = version + 1",
			k,
		)
		if err != nil {
			log.Logger.Errorf("refresh tacacs_meta key=%s failed: %v", k, err)
			return err
		}
	}
	return nil
}
