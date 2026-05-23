package db

import (
	"tacacs/pkg/public/log"
)

func GetTacacsAdminUser() (users []string) {
	users = make([]string, 0)
	selectSql := "SELECT user FROM tacacs_admin"
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
