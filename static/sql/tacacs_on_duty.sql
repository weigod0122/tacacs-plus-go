CREATE TABLE `tacacs_on_duty` (
    `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键自增ID',
    `user` varchar(30) NOT NULL COMMENT '值班用户名',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_user` (`user`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='当前值班用户列表，值班用户认证时跳过角色检查';
