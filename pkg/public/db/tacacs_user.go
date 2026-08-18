package db

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
	"tacacs/pkg/public/cfg"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/utils"
	"time"
)

type TacacsUser struct {
	User               string `db:"user"`
	PhoneNumber        string `db:"phone_number"`
	Email              string `db:"email"`
	CreateTime         string `db:"create_time"`
	Role               string `db:"role"`
	RoleUpdateTime     string `db:"role_update_time"`
	Password           string `db:"password"`
	PasswordUpdateTime string `db:"password_update_time"`
	Status             string `db:"status"`
	StatusUpdateTime   string `db:"status_update_time"`
	Notes              string `db:"notes"`
}

// tacacsUserCols 是显式列清单,顺序必须与 Scan 调用一一对应。
// 不用 SELECT * 的原因:DBA 加列(如自增 id / 业务字段)时,SELECT * 会让
// rows.Scan 因目标数量不匹配直接报错,而显式列名不受新列影响。
const tacacsUserCols = "user, phone_number, email, create_time, role, role_update_time, password, password_update_time, status, status_update_time, notes"

func GetTacacsUserInfos() (tacacsUsers []*TacacsUser, err error) {
	selectSql := "SELECT " + tacacsUserCols + " FROM tacacs_user"
	rows, err := DbRead.Query(selectSql)
	if err != nil {
		log.Logger.Errorf("select TacacsUser from tacacs_user database is failed, because %v", err)
		return tacacsUsers, err
	}
	defer func() {
		rows.Close()
	}()
	for rows.Next() {
		i := &TacacsUser{}
		err = rows.Scan(&i.User, &i.PhoneNumber, &i.Email, &i.CreateTime, &i.Role, &i.RoleUpdateTime, &i.Password, &i.PasswordUpdateTime, &i.Status, &i.StatusUpdateTime, &i.Notes)
		if err != nil {
			log.Logger.Errorf("read database data is failed ,because %v", err)
			return tacacsUsers, err
		}
		tacacsUsers = append(tacacsUsers, i)
	}
	return tacacsUsers, nil
}

func GetTacacsUserInfoByUserName(userName string) (tacacsUsers *TacacsUser, err error) {
	selectSql := "SELECT " + tacacsUserCols + " FROM tacacs_user where user = ?"
	rows, err := DbRead.Query(selectSql, userName)
	if err != nil {
		log.Logger.Errorf("select TacacsUser from tacacs_user database is failed, because %v", err)
		return tacacsUsers, err
	}
	defer func() {
		rows.Close()
	}()
	for rows.Next() {
		i := &TacacsUser{}
		err = rows.Scan(&i.User, &i.PhoneNumber, &i.Email, &i.CreateTime, &i.Role, &i.RoleUpdateTime, &i.Password, &i.PasswordUpdateTime, &i.Status, &i.StatusUpdateTime, &i.Notes)
		if err != nil {
			log.Logger.Errorf("read database data is failed ,because %v", err)
			return tacacsUsers, err
		}
		tacacsUsers = i
	}
	return tacacsUsers, nil
}

func GetTacacsUser() (users []string) {
	tacacsUsers, err := GetTacacsUserInfos()
	if err != nil {
		return nil
	}
	for _, u := range tacacsUsers {
		users = append(users, u.User)
	}
	return
}

func UpdateRoleByName(user, role string) {
	now := time.Now().Format("2006-01-02 15:04:05")
	updateSql := "UPDATE tacacs_user SET role = ?, role_update_time = ? where user = ?"
	_, err := DbWrite.Exec(updateSql, role, now, user)
	if err != nil {
		log.Logger.Errorf("更新用户（%v）权限失败: %v", user, err)
		return
	}
	BumpMetaVersion(MetaKeyUser)
}

func CreateUser(user, PhoneNumber, email, password, notes string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	insertSql := "INSERT INTO tacacs_user(user, phone_number, email, create_time, role, role_update_time, password, password_update_time, status, status_update_time, notes) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	passwordHash, err := utils.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	_, err = DbWrite.Exec(insertSql, user, PhoneNumber, email, now, "null", now, passwordHash, now, "1", now, notes)
	if err != nil {
		log.Logger.Errorf("DB Exec err%v", err)
		return err
	}
	BumpMetaVersion(MetaKeyUser)
	return nil
}

func UpdateUserPassword(user, password string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	passwordHash, err := utils.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	updateSql := "UPDATE tacacs_user SET password = ?, password_update_time = ? WHERE user = ?"
	result, err := DbWrite.Exec(updateSql, passwordHash, now, user)
	if err != nil {
		log.Logger.Errorf("DB Exec err%v", err)
		return err
	}
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("user %q does not exist", user)
	}
	BumpMetaVersion(MetaKeyUser)
	return nil
}

// ResetUserPassword atomically restores a user's status and changes the
// password.  Reset used to issue two independent UPDATEs; a client polling a
// read replica could observe the first metadata bump with the old password and
// cache that inconsistent snapshot.  Keeping the row update and the
// application-level metadata bump in one transaction makes the snapshot
// visible to replicas only after both values are committed.
func ResetUserPassword(user, password string) error {
	passwordHash, err := utils.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if DbWrite == nil {
		return fmt.Errorf("write database is not initialized")
	}

	tx, err := DbWrite.Begin()
	if err != nil {
		return fmt.Errorf("begin reset password transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().Format("2006-01-02 15:04:05")
	result, err := tx.Exec(
		"UPDATE tacacs_user SET status = ?, status_update_time = ?, password = ?, password_update_time = ? WHERE user = ?",
		"1", now, passwordHash, now, user,
	)
	if err != nil {
		return fmt.Errorf("update user reset fields: %w", err)
	}
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("user %q does not exist", user)
	}

	// In trigger mode the AFTER UPDATE trigger performs this bump in the same
	// transaction.  In triggerless mode the server owns the bump instead.
	if conf := cfg.ServerConfig(); conf != nil && !conf.DatabaseTriggers {
		if _, err = tx.Exec(
			"INSERT INTO tacacs_meta (k, version) VALUES (?, 1) ON DUPLICATE KEY UPDATE version = version + 1",
			MetaKeyUser,
		); err != nil {
			return fmt.Errorf("bump user metadata version: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit reset password transaction: %w", err)
	}
	committed = true
	return nil
}

func UpdateUserNotes(user, notes string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	updateSql := "UPDATE tacacs_user SET notes = ?, password_update_time = ? WHERE user = ?"
	_, err := DbWrite.Exec(updateSql, notes, now, user)
	if err != nil {
		log.Logger.Errorf("DB Exec err%v", err)
		return err
	}
	BumpMetaVersion(MetaKeyUser)
	return nil
}

func UpdateUserStatus(user, status string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	updateSql := "UPDATE tacacs_user SET status = ?, status_update_time = ? WHERE user = ?"
	_, err := DbWrite.Exec(updateSql, status, now, user)
	if err != nil {
		log.Logger.Errorf("DB Exec err%v", err)
		return err
	}
	BumpMetaVersion(MetaKeyUser)
	return nil
}

// BatchUpdateUserRole 单事务批量回写多个用户的 role + role_update_time。
// 走 SQL 的 CASE WHEN 一条语句搞定，不论多少用户都只 1 次 round-trip。
// 跳过 roleMap 为空 / nil 的情况。所有用户必须事先存在；不存在的不会被 INSERT。
func BatchUpdateUserRole(roleMap map[string]string) error {
	if len(roleMap) == 0 {
		return nil
	}
	now := time.Now().Format("2006-01-02 15:04:05")

	var sb strings.Builder
	sb.WriteString("UPDATE tacacs_user SET role = CASE user")
	args := make([]interface{}, 0, len(roleMap)*3+1)
	users := make([]string, 0, len(roleMap))
	for user, role := range roleMap {
		sb.WriteString(" WHEN ? THEN ?")
		args = append(args, user, role)
		users = append(users, user)
	}
	sb.WriteString(" END, role_update_time = ? WHERE user IN (")
	args = append(args, now)
	for i, user := range users {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("?")
		args = append(args, user)
	}
	sb.WriteString(")")

	_, err := DbWrite.Exec(sb.String(), args...)
	if err != nil {
		log.Logger.Errorf("BatchUpdateUserRole failed: %v (size=%d)", err, len(roleMap))
		return err
	}
	BumpMetaVersion(MetaKeyUser)
	return nil
}

// BatchUpdateUserStatus 把一批用户改到同一个 status，同时刷新 status_update_time。
// 跳过空切片。
func BatchUpdateUserStatus(users []string, status string) error {
	if len(users) == 0 {
		return nil
	}
	now := time.Now().Format("2006-01-02 15:04:05")

	placeholders := strings.Repeat("?,", len(users))
	placeholders = placeholders[:len(placeholders)-1]

	updateSql := fmt.Sprintf(
		"UPDATE tacacs_user SET status = ?, status_update_time = ? WHERE user IN (%s)",
		placeholders,
	)
	args := make([]interface{}, 0, len(users)+2)
	args = append(args, status, now)
	for _, user := range users {
		args = append(args, user)
	}

	_, err := DbWrite.Exec(updateSql, args...)
	if err != nil {
		log.Logger.Errorf("BatchUpdateUserStatus failed: %v (size=%d, status=%s)", err, len(users), status)
		return err
	}
	BumpMetaVersion(MetaKeyUser)
	return nil
}

func UpdateUserEmailAndPhoneNumber(user, email, phoneNumber string) error {
	updateSql := "UPDATE tacacs_user SET email = ?, phone_number = ? WHERE user = ?"
	_, err := DbWrite.Exec(updateSql, email, phoneNumber, user)
	if err != nil {
		log.Logger.Errorf("DB Exec err%v", err)
		return err
	}
	BumpMetaVersion(MetaKeyUser)
	return nil
}

func GetTacacsUserData2Md5() (string, error) {
	selectSql := "SELECT CONCAT(user,create_time,role,role_update_time,password,password_update_time,status,status_update_time,notes) AS data_string FROM tacacs_user"
	rows, err := DbRead.Query(selectSql)
	if err != nil {
		log.Logger.Errorf("select TacacsApproval from database is failed, because %v", err)
		return "", err
	}
	defer rows.Close()

	var allData strings.Builder
	for rows.Next() {
		var dataString string
		if err := rows.Scan(&dataString); err != nil {
			log.Logger.Errorf("scan row data failed, because %v", err)
			return "", err
		}
		allData.WriteString(dataString)
	}

	if err = rows.Err(); err != nil {
		log.Logger.Errorf("error occurred while iterating rows, because %v", err)
		return "", err
	}

	md5Hash := md5.Sum([]byte(allData.String()))
	return hex.EncodeToString(md5Hash[:]), nil
}
