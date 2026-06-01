-- tacacs_misc:杂项配置 K/V 表
-- 用途:存储无法独立成表、又无需触发缓存失效的运维参数
--      (当前承载外部日志系统跳转 URL 与可见性开关;未来可承载其他单值/短文本设置)。
-- 行级 schema:每条记录一个 (k, v) 键值对,v 用 VARCHAR(2048) 兜底任意 URL/短文本;
--             description 是给 DBA 看的"这一行是干什么用的"自述。
--
-- 权威源:description 由 Go 代码 (pkg/public/db/misc.go::MiscDescriptions) 维护,
--        server 启动时调 SyncMiscDescriptions 把每个 key 的 description 写入/校正
--        到 DB(行不存在则连同 v='' 一起插入;行存在但 description 跟代码不一致
--        则只更新 description,不动 v)。本 SQL 文件不再 seed 任何数据。
--
-- 整脚本可重复执行(CREATE TABLE IF NOT EXISTS + ADD COLUMN IF NOT EXISTS):
-- 空库首跑会建表,之后再跑都是 no-op。
-- 执行账号需要:CREATE / ALTER 权限。MySQL 8.0.29+ (依赖 ADD COLUMN IF NOT EXISTS)。

CREATE TABLE IF NOT EXISTS `tacacs_misc` (
    `id`          BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键自增ID',
    `k`           VARCHAR(64) NOT NULL COMMENT '配置项 key(如 log_redirect_url_authen)',
    `v`           VARCHAR(2048) NOT NULL DEFAULT '' COMMENT '配置项 value,UTF-8 文本,URL/短 JSON 均可',
    `description` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '该 key 的用途说明,由 Go 代码启动时同步,业务运行不写',
    `updated_at`  TIMESTAMP(6) NOT NULL
                  DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6)
                  COMMENT '最后更新时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_k` (`k`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='tacacs 杂项配置 K/V';
