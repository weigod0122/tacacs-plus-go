CREATE TABLE `tacacs_command_template` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `template` varchar(50) NOT NULL COMMENT '模版名字',
    `command_template` varchar(300) NOT NULL,
    PRIMARY KEY (`id`),
    KEY `tacacs_command_template_template_index` (`template`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COMMENT='定义各种权限的可使用命令集'

