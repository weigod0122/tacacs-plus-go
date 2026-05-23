CREATE TABLE `tacacs_user` (
    `user` varchar(30) NOT NULL COMMENT '用户名',
    `create_time` varchar(30) NOT NULL COMMENT '创建时间',
    `role` varchar(30) NOT NULL COMMENT '所属成员组',
    `role_update_time` varchar(30) NOT NULL COMMENT '角色最近更新时间',
    `password` varchar(80) NOT NULL COMMENT '密码',
    `password_update_time` varchar(30) NOT NULL COMMENT '密码最近更新时间',
    `status` varchar(2) NOT NULL DEFAULT '1' COMMENT '状态标记，0为已删除，1为正常，2为暂停使用',
    `status_update_time` varchar(30) NOT NULL COMMENT '状态最近更新时间',
    `notes` text,
    PRIMARY KEY (`user`),
    KEY `user` (`user`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 ROW_FORMAT=DYNAMIC

