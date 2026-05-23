CREATE TABLE `tacacs_approval` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `create_time` datetime NOT NULL,
    `user` varchar(50) NOT NULL,
    `approval_permissions` varchar(50) NOT NULL,
    `start_time` datetime NOT NULL,
    `end_time` datetime NOT NULL,
    `status` int NOT NULL COMMENT '4=通过；3=审批中；2=审批不通过；1=工单超时关闭；0=工单手动关闭',
    `approver` varchar(30) DEFAULT NULL COMMENT '审批人（status=2/4 时记录）',
    `approve_time` datetime DEFAULT NULL COMMENT '审批时间',
    PRIMARY KEY (`id`),
    KEY `tacacs_approval_login_approval_permissions_index` (`approval_permissions`),
    KEY `tacacs_approval_login_status_index` (`status`),
    KEY `tacacs_approval_login_user_index` (`user`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3

-- 升级老库：
-- ALTER TABLE tacacs_approval ADD COLUMN approver VARCHAR(30) DEFAULT NULL COMMENT '审批人';
-- ALTER TABLE tacacs_approval ADD COLUMN approve_time DATETIME DEFAULT NULL COMMENT '审批时间';

