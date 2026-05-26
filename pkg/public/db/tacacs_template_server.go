package db

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/utils"
)

type TacacsServerTemplate struct {
	ID             int64  `db:"id"`
	Template       string `db:"template"`
	ServerTemplate string `db:"server_template"`
}

// tacacsServerTemplateCols 必须与下面 Scan 顺序一一对应。
// 不用 SELECT *,避免 DBA 加列后 Scan 数量不匹配直接报错。
const tacacsServerTemplateCols = "id, template, server_template"

func GetTacacsServerTemplatesByTemplate(template string) (tacacsServerTemplates []*TacacsServerTemplate, err error) {
	rows, err := DbRead.Query("SELECT "+tacacsServerTemplateCols+" FROM tacacs_server_template WHERE template = ?", template)
	if err != nil {
		log.Logger.Errorf("select TacacsServerTemplate from tacacs_server_template database is failed, because %v", err)
		return tacacsServerTemplates, err
	}
	defer func() {
		rows.Close()
	}()
	for rows.Next() {
		i := &TacacsServerTemplate{}
		err = rows.Scan(&i.ID, &i.Template, &i.ServerTemplate)
		if err != nil {
			log.Logger.Errorf("read database data is failed ,because %v", err)
			return tacacsServerTemplates, err
		}
		tacacsServerTemplates = append(tacacsServerTemplates, i)
	}
	return tacacsServerTemplates, nil
}

func GetTacacsServerTemplates() (tacacsServerTemplates []*TacacsServerTemplate, err error) {
	selectSql := "SELECT " + tacacsServerTemplateCols + " FROM tacacs_server_template"
	rows, err := DbRead.Query(selectSql)
	if err != nil {
		log.Logger.Errorf("select TacacsServerTemplate from tacacs_server_template database is failed, because %v", err)
		return tacacsServerTemplates, err
	}
	defer func() {
		rows.Close()
	}()
	for rows.Next() {
		i := &TacacsServerTemplate{}
		err = rows.Scan(&i.ID, &i.Template, &i.ServerTemplate)
		if err != nil {
			log.Logger.Errorf("read database data is failed ,because %v", err)
			return tacacsServerTemplates, err
		}
		tacacsServerTemplates = append(tacacsServerTemplates, i)
	}
	return tacacsServerTemplates, nil
}

func AddTacacsServerTemplates(template string, serverTemplate []string) error {
	insertSql := "INSERT INTO tacacs_server_template (template, server_template) VALUES (?, ?)"
	inserted := 0
	for _, srv := range utils.UniqueStringSlice(serverTemplate) {
		if _, err := DbWrite.Exec(insertSql, template, srv); err != nil {
			log.Logger.Errorf("DB Exec err: %v", err)
			return err
		}
		inserted++
	}
	if inserted > 0 {
		BumpMetaVersion(MetaKeyServer)
	}
	return nil
}

func IsIdExistsInTacacsServerTemplate(id int64) bool {
	var template TacacsServerTemplate
	err := DbRead.QueryRow("SELECT id FROM tacacs_server_template WHERE id = ?", id).Scan(&template.ID)
	return err == nil
}

func IsTemplateExistsInTacacsServerTemplate(Template string) bool {
	var template TacacsServerTemplate
	err := DbRead.QueryRow("SELECT id FROM tacacs_server_template WHERE template = ?", Template).Scan(&template.Template)
	return err == nil
}

func DelTacacsServerTemplate(item interface{}) error {
	switch v := item.(type) {
	case int64:
		_, err := DbWrite.Exec("DELETE FROM tacacs_server_template WHERE id = ?", v)
		if err != nil {
			log.Logger.Errorf("delete TacacsServerTemplate failed, error: %v", err)
			return err
		}
	case string:
		_, err := DbWrite.Exec("DELETE FROM tacacs_server_template WHERE template = ?", v)
		if err != nil {
			log.Logger.Errorf("delete TacacsServerTemplate failed, error: %v", err)
			return err
		}
	default:
		return fmt.Errorf("unsupported type for deletion: %T", item)
	}
	BumpMetaVersion(MetaKeyServer)
	return nil
}

func IsIdsTemplateOnlyServer(id int64) (string, bool) {
	var template TacacsServerTemplate
	err := DbRead.QueryRow("SELECT template FROM tacacs_server_template WHERE id = ?", id).Scan(&template.Template)
	if err != nil {
		return template.Template, true
	}
	return template.Template, IsTemplateOnlyServer(template.Template)
}

func IsTemplateOnlyServer(template string) bool {
	var count int64
	err := DbRead.QueryRow("SELECT count(*) FROM tacacs_server_template WHERE template = ?", template).Scan(&count)
	if err != nil {
		return true
	}
	if count > 1 {
		return false
	}
	return true
}

func IsIdOrTemplateDeleteServer(item interface{}) (b bool) {
	switch v := item.(type) {
	case int64:
		template, is := IsIdsTemplateOnlyServer(v)
		if template == "" {
			return false
		}
		//是唯一的一个模版的话就需要判断是否有角色引用，有引用就不能删；不是唯一的就随便删
		if is {
			list := GetTacacsRoleTemplateByServerTemplate(template)
			if len(list) < 1 { //没有引用
				b = true
			} else { //有引用
				b = false
			}
		} else {
			b = true
		}
	case string:
		if IsTemplateOnlyServer(v) {
			list := GetTacacsRoleTemplateByServerTemplate(v)
			if len(list) < 1 { //没有引用
				b = true
			} else { //有引用
				b = false
			}
		} else {
			b = true
		}
	default:
		b = false
	}

	return b
}

func GetTemplateServerInUsed() ([]string, []int64) {
	var templateServerInUsed []string
	var templateIdServerInUsed []int64
	inUsedRoles := GetInUsedRole()
	for _, inUsedRole := range inUsedRoles {
		inUsedRolesCmd := GetTacacsRoleTemplateByTemplate(inUsedRole).ServerTemplateList
		if !utils.IsValueInList(inUsedRolesCmd, templateServerInUsed) {
			templateServerInUsed = append(templateServerInUsed, inUsedRolesCmd)
		}
	}

	tacacsCommandTemplates, _ := GetTacacsServerTemplates()
	for _, tacacsCommandTemplate := range tacacsCommandTemplates {
		if utils.IsValueInList(tacacsCommandTemplate.Template, templateServerInUsed) {
			templateIdServerInUsed = append(templateIdServerInUsed, tacacsCommandTemplate.ID)
		}
	}

	return templateServerInUsed, templateIdServerInUsed
}

func IsTemplateServerInUsed(item interface{}) (b bool) {
	nameInUsed, idInUsed := GetTemplateServerInUsed()

	switch v := item.(type) {
	case int64:
		b = utils.IsValueInList(v, idInUsed)
	case string:
		b = utils.IsValueInList(v, nameInUsed)
	}

	return
}

func GetTacacsTemplateServerData2Md5() (string, error) {
	selectSql := "SELECT CONCAT(id, template, server_template) AS data_string FROM tacacs_server_template"
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
