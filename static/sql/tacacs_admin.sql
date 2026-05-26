CREATE TABLE `tacacs_admin` (
    `id` bigint(20) NOT NULL AUTO_INCREMENT,
    `user` varchar(30) NOT NULL,
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='tacacs系统管理员';

