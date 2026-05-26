CREATE TABLE `tacacs_user` (
    `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键自增ID',
    `user` varchar(30) NOT NULL COMMENT '用户名',
    `phone_number` varchar(20) NOT NULL DEFAULT '' COMMENT '手机号',
    `email` varchar(100) NOT NULL DEFAULT '' COMMENT '邮箱',
    `create_time` varchar(30) NOT NULL COMMENT '创建时间',
    `role` varchar(30) NOT NULL COMMENT '所属成员组',
    `role_update_time` varchar(30) NOT NULL COMMENT '角色最近更新时间',
    `password` varchar(80) NOT NULL COMMENT '密码',
    `password_update_time` varchar(30) NOT NULL COMMENT '密码最近更新时间',
    `status` varchar(2) NOT NULL DEFAULT '1' COMMENT '状态标记，0为已删除，1为正常，2为暂停使用',
    `status_update_time` varchar(30) NOT NULL COMMENT '状态最近更新时间',
    `notes` text,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_user` (`user`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC COMMENT='tacacs系统用户信息';
