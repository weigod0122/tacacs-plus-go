-- tacacs_meta：缓存失效版本号表
-- 用途：替代 information_schema.UPDATE_TIME 作为缓存变更检测来源。
-- 每次被监控表发生 INSERT/UPDATE/DELETE，对应的 version 自增 1。
-- Go 侧 GetTablesUpdateTime() 拼接所有 version 形成 key，发现差异即重建缓存。
--
-- 整脚本可重复执行（CREATE TABLE IF NOT EXISTS / ON DUPLICATE KEY / DROP TRIGGER IF EXISTS）。
-- 执行账号需要：CREATE, INSERT, UPDATE, TRIGGER 权限。

CREATE TABLE IF NOT EXISTS `tacacs_meta` (
    `id`         BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键自增ID',
    `k`          VARCHAR(32) NOT NULL,
    `version`    BIGINT UNSIGNED NOT NULL DEFAULT 0,
    `updated_at` TIMESTAMP(6) NOT NULL
                 DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_k` (`k`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='tacacs 缓存失效版本号';

-- 幂等播种 6 行（与 Go 侧 order 严格对应：user/role/server/command/on_duty/approval）
INSERT INTO `tacacs_meta` (`k`) VALUES
    ('user'), ('role'), ('server'), ('command'), ('on_duty'), ('approval')
ON DUPLICATE KEY UPDATE `k` = VALUES(`k`);

DELIMITER $$

-- ============ tacacs_user ============
DROP TRIGGER IF EXISTS `trg_tacacs_user_ai`$$
CREATE TRIGGER `trg_tacacs_user_ai` AFTER INSERT ON `tacacs_user`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('user', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

DROP TRIGGER IF EXISTS `trg_tacacs_user_au`$$
CREATE TRIGGER `trg_tacacs_user_au` AFTER UPDATE ON `tacacs_user`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('user', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

DROP TRIGGER IF EXISTS `trg_tacacs_user_ad`$$
CREATE TRIGGER `trg_tacacs_user_ad` AFTER DELETE ON `tacacs_user`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('user', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

-- ============ tacacs_role_template ============
DROP TRIGGER IF EXISTS `trg_tacacs_role_ai`$$
CREATE TRIGGER `trg_tacacs_role_ai` AFTER INSERT ON `tacacs_role_template`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('role', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

DROP TRIGGER IF EXISTS `trg_tacacs_role_au`$$
CREATE TRIGGER `trg_tacacs_role_au` AFTER UPDATE ON `tacacs_role_template`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('role', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

DROP TRIGGER IF EXISTS `trg_tacacs_role_ad`$$
CREATE TRIGGER `trg_tacacs_role_ad` AFTER DELETE ON `tacacs_role_template`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('role', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

-- ============ tacacs_server_template ============
DROP TRIGGER IF EXISTS `trg_tacacs_server_ai`$$
CREATE TRIGGER `trg_tacacs_server_ai` AFTER INSERT ON `tacacs_server_template`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('server', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

DROP TRIGGER IF EXISTS `trg_tacacs_server_au`$$
CREATE TRIGGER `trg_tacacs_server_au` AFTER UPDATE ON `tacacs_server_template`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('server', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

DROP TRIGGER IF EXISTS `trg_tacacs_server_ad`$$
CREATE TRIGGER `trg_tacacs_server_ad` AFTER DELETE ON `tacacs_server_template`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('server', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

-- ============ tacacs_command_template ============
DROP TRIGGER IF EXISTS `trg_tacacs_command_ai`$$
CREATE TRIGGER `trg_tacacs_command_ai` AFTER INSERT ON `tacacs_command_template`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('command', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

DROP TRIGGER IF EXISTS `trg_tacacs_command_au`$$
CREATE TRIGGER `trg_tacacs_command_au` AFTER UPDATE ON `tacacs_command_template`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('command', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

DROP TRIGGER IF EXISTS `trg_tacacs_command_ad`$$
CREATE TRIGGER `trg_tacacs_command_ad` AFTER DELETE ON `tacacs_command_template`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('command', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

-- ============ tacacs_on_duty ============
DROP TRIGGER IF EXISTS `trg_tacacs_on_duty_ai`$$
CREATE TRIGGER `trg_tacacs_on_duty_ai` AFTER INSERT ON `tacacs_on_duty`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('on_duty', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

DROP TRIGGER IF EXISTS `trg_tacacs_on_duty_au`$$
CREATE TRIGGER `trg_tacacs_on_duty_au` AFTER UPDATE ON `tacacs_on_duty`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('on_duty', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

DROP TRIGGER IF EXISTS `trg_tacacs_on_duty_ad`$$
CREATE TRIGGER `trg_tacacs_on_duty_ad` AFTER DELETE ON `tacacs_on_duty`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('on_duty', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

-- ============ tacacs_approval ============
DROP TRIGGER IF EXISTS `trg_tacacs_approval_ai`$$
CREATE TRIGGER `trg_tacacs_approval_ai` AFTER INSERT ON `tacacs_approval`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('approval', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

DROP TRIGGER IF EXISTS `trg_tacacs_approval_au`$$
CREATE TRIGGER `trg_tacacs_approval_au` AFTER UPDATE ON `tacacs_approval`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('approval', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

DROP TRIGGER IF EXISTS `trg_tacacs_approval_ad`$$
CREATE TRIGGER `trg_tacacs_approval_ad` AFTER DELETE ON `tacacs_approval`
FOR EACH ROW
    INSERT INTO `tacacs_meta` (`k`, `version`) VALUES ('approval', 1)
    ON DUPLICATE KEY UPDATE `version` = `version` + 1$$

DELIMITER ;
