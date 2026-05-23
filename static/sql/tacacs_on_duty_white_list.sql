CREATE TABLE `tacacs_on_duty_white_list` (
    `user` varchar(30) NOT NULL COMMENT '值班白名单用户名',
    PRIMARY KEY (`user`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='允许被加入值班列表的用户白名单'
