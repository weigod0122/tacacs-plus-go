CREATE TABLE `tacacs_role_template` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `template` varchar(50) NOT NULL COMMENT '模版名字',
    `server_template_list` varchar(300) NOT NULL,
    `command_template_list` varchar(300) NOT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_template_index` (`template`),
    KEY `idx_server_template_list_index` (`server_template_list`),
    KEY `idx_command_template_list_index` (`command_template_list`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='定义各种角色的可使用源服务器模版和命令模版';

