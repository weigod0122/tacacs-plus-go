CREATE TABLE `tacacs_command_template` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `template` varchar(50) NOT NULL COMMENT '模版名字',
    `command_template` varchar(300) NOT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_template_index` (`template`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='定义各种权限的可使用命令集';

