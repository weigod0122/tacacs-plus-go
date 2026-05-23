CREATE TABLE `tacacs_on_duty` (
    `user` varchar(30) NOT NULL COMMENT '值班用户名',
    PRIMARY KEY (`user`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='当前值班用户列表，值班用户认证时跳过角色检查'
