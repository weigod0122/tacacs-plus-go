CREATE TABLE `tacacs_server_template` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `template` varchar(50) NOT NULL COMMENT '模版名字',
    `server_template` varchar(300) NOT NULL,
    PRIMARY KEY (`id`),
    KEY `tacacs_server_template_template_index` (`template`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COMMENT='定义各种模版的可使用源服务器'

