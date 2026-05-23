package db

import (
	"strconv"
	"strings"
)

// GetTablesUpdateTime 返回 tacacs_meta 中各被监控表的版本号拼接串，
// 供调用方与上次返回值做字符串等值比较以判断是否需要重建缓存。
//
// 顺序：user / role / server / command / on_duty / approval，
// 缺失行视为 0。任一 INSERT/UPDATE/DELETE（包括外部 SQL）都会经触发器
// bump tacacs_meta.version，从而改变返回值。
func GetTablesUpdateTime() (string, error) {
	rows, err := DbRead.Query("SELECT k, version FROM tacacs_meta")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	versions := make(map[string]uint64, 6)
	for rows.Next() {
		var k string
		var v uint64
		if err := rows.Scan(&k, &v); err != nil {
			return "", err
		}
		versions[k] = v
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	order := []string{"user", "role", "server", "command", "on_duty", "approval"}
	parts := make([]string, 0, len(order))
	for _, k := range order {
		parts = append(parts, strconv.FormatUint(versions[k], 10))
	}
	return strings.Join(parts, "_"), nil
}
