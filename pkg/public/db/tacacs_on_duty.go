package db

import (
	"fmt"
	"strings"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/utils"
)

// 获取tacacs_on_duty表当前数据
func GetTacacsOnDutyUser() (users []string) {
	users = make([]string, 0)
	selectSql := "SELECT user FROM tacacs_on_duty"
	rows, err := DbRead.Query(selectSql)
	if err != nil {
		log.Logger.Errorf("select from tacacs_admin database is failed, because %v", err)
		return
	}
	defer func() {
		rows.Close()
	}()
	for rows.Next() {
		var i string
		err = rows.Scan(&i)
		if err != nil {
			log.Logger.Errorf("read database data is failed ,because %v", err)
			return
		}
		users = append(users, i)
	}
	return users
}

// 获取tacacs_on_duty_white_list表数据
func GetTacacsOnDutyUserWhiteList() ([]string, error) {
	users := make([]string, 0)
	selectSql := "SELECT user FROM tacacs_on_duty_white_list"
	rows, err := DbRead.Query(selectSql)
	if err != nil {
		return users, fmt.Errorf("select from tacacs_admin database is failed, because %v", err)
	}
	defer func() {
		rows.Close()
	}()
	for rows.Next() {
		var i string
		err = rows.Scan(&i)
		if err != nil {
			return users, fmt.Errorf("read database data is failed ,because %v", err)
		}
		users = append(users, i)
	}
	return users, nil
}

// 覆盖tacacs_on_duty表，返回增加和删除的人员列表
func CoverTacacsOnDutyUser(onDutyUsers []string) ([]string, []string, error) {
	inDbUsers := GetTacacsOnDutyUser()

	needAdd, needDelete := make([]string, 0), make([]string, 0)

	needNotUpdateUser := utils.GetIntersection(onDutyUsers, inDbUsers)

	for _, onDutyUser := range onDutyUsers {
		if b, ok := needNotUpdateUser[onDutyUser]; !ok || !b {
			needAdd = append(needAdd, onDutyUser)
		}
	}
	for _, inDbUser := range inDbUsers {
		if b, ok := needNotUpdateUser[inDbUser]; !ok || !b {
			needDelete = append(needDelete, inDbUser)
		}
	}

	log.Logger.Infof("need not update on duty user: %v", needNotUpdateUser)

	if len(needDelete) > 0 {
		log.Logger.Infof("need delete on duty user: %v", needDelete)
		placeholders := strings.Repeat("?,", len(needDelete))
		placeholders = strings.TrimSuffix(placeholders, ",")

		args := make([]interface{}, len(needDelete))
		for i, user := range needDelete {
			args[i] = user
		}

		_, err := DbWrite.Exec(fmt.Sprintf("DELETE FROM tacacs_on_duty WHERE user IN (%s)", placeholders), args...)
		if err != nil {
			return nil, nil, err
		}
	}

	if len(needAdd) > 0 {
		log.Logger.Infof("need add on duty user: %v", needAdd)
		placeholders := strings.Repeat("(?),", len(needAdd))
		placeholders = strings.TrimSuffix(placeholders, ",")

		args := make([]interface{}, len(needAdd))
		for i, user := range needAdd {
			args[i] = user
		}

		_, err := DbWrite.Exec(fmt.Sprintf("INSERT INTO tacacs_on_duty (user) VALUES %s", placeholders), args...)
		if err != nil {
			return nil, nil, err
		}
	}

	return needAdd, needDelete, nil
}
