CREATE TABLE `tacacs_on_duty_white_list` (
    `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键自增ID',
    `user` varchar(30) NOT NULL COMMENT '值班白名单用户名',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_user` (`user`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='允许被加入值班列表的用户白名单';
