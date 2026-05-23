CREATE TABLE `tacacs_admin` (
    `id` bigint(20) NOT NULL AUTO_INCREMENT,
    `user` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL,
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='tacacs系统管理员'

-- 如果之前因接入飞书回调加过 feishu_open_id 字段，回退它（内网部署不需要）：
-- ALTER TABLE tacacs_admin DROP COLUMN feishu_open_id;
