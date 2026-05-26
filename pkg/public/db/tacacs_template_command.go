package db

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/utils"
)

type TacacsCommandTemplate struct {
	ID              int64  `db:"id"`
	Template        string `db:"template"`
	CommandTemplate string `db:"command_template"`
}

// tacacsCommandTemplateCols 必须与下面 Scan 顺序一一对应。
// 不用 SELECT *,避免 DBA 加列后 Scan 数量不匹配直接报错。
const tacacsCommandTemplateCols = "id, template, command_template"

func GetTacacsCommandTemplatesByTemplate(template string) (tacacsCommandTemplates []*TacacsCommandTemplate, err error) {
	rows, err := DbRead.Query("SELECT "+tacacsCommandTemplateCols+" FROM tacacs_command_template WHERE template = ?", template)
	if err != nil {
		log.Logger.Errorf("select TacacsCommandTemplate from tacacs_command_template database is failed, because %v", err)
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	for rows.Next() {
		i := &TacacsCommandTemplate{}
		err = rows.Scan(&i.ID, &i.Template, &i.CommandTemplate)
		if err != nil {
			log.Logger.Errorf("read database data is failed ,because %v", err)
			return nil, err
		}
		tacacsCommandTemplates = append(tacacsCommandTemplates, i)
	}
	return tacacsCommandTemplates, nil
}

func GetTacacsCommandTemplates() (tacacsCommandTemplates []*TacacsCommandTemplate, err error) {
	selectSql := "SELECT " + tacacsCommandTemplateCols + " FROM tacacs_command_template"
	rows, err := DbRead.Query(selectSql)
	if err != nil {
		log.Logger.Errorf("select TacacsCommandTemplate from tacacs_command_template database is failed, because %v", err)
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	for rows.Next() {
		i := &TacacsCommandTemplate{}
		err = rows.Scan(&i.ID, &i.Template, &i.CommandTemplate)
		if err != nil {
			log.Logger.Errorf("read database data is failed ,because %v", err)
			return nil, err
		}
		tacacsCommandTemplates = append(tacacsCommandTemplates, i)
	}
	return tacacsCommandTemplates, nil
}

func AddTacacsCommandTemplate(template string, commandTemplate []string) error {
	insertSql := "INSERT INTO tacacs_command_template (template, command_template) VALUES (?, ?)"
	inserted := 0
	for _, cmd := range utils.UniqueStringSlice(commandTemplate) {
		if _, err := DbWrite.Exec(insertSql, template, cmd); err != nil {
			log.Logger.Errorf("DB Exec err: %v", err)
			return err
		}
		inserted++
	}
	if inserted > 0 {
		BumpMetaVersion(MetaKeyCommand)
	}
	return nil
}

func IsIdExistsInTacacsCommandTemplate(id int64) bool {
	var template TacacsCommandTemplate
	err := DbRead.QueryRow("SELECT id FROM tacacs_command_template WHERE id = ?", id).Scan(&template.ID)
	return err == nil
}

func IsTemplateExistsInTacacsCommandTemplate(Template string) bool {
	var template TacacsCommandTemplate
	err := DbRead.QueryRow("SELECT id FROM tacacs_command_template WHERE template = ?", Template).Scan(&template.Template)
	return err == nil
}

func DelTacacsCommandTemplate(item interface{}) error {
	switch v := item.(type) {
	case int64:
		_, err := DbWrite.Exec("DELETE FROM tacacs_command_template WHERE id = ?", v)
		if err != nil {
			log.Logger.Errorf("delete TacacsCommandTemplate failed, error: %v", err)
			return err
		}
	case string:
		_, err := DbWrite.Exec("DELETE FROM tacacs_command_template WHERE template = ?", v)
		if err != nil {
			log.Logger.Errorf("delete TacacsCommandTemplate failed, error: %v", err)
			return err
		}
	default:
		return fmt.Errorf("unsupported type for deletion: %T", item)
	}
	BumpMetaVersion(MetaKeyCommand)
	return nil
}

func IsIdsTemplateOnlyCommand(id int64) (string, bool) {
	var template TacacsCommandTemplate
	err := DbRead.QueryRow("SELECT template FROM tacacs_command_template WHERE id = ?", id).Scan(&template.Template)
	if err != nil {
		return template.Template, true
	}
	return template.Template, IsTemplateOnlyCommand(template.Template)
}

func IsTemplateOnlyCommand(template string) bool {
	var count int64
	err := DbRead.QueryRow("SELECT count(*) FROM tacacs_command_template WHERE template = '?'", template).Scan(&count)
	if err != nil {
		return true
	}
	if count > 1 {
		return false
	}
	return true
}

func IsIdOrTemplateDeleteCommand(item interface{}) (b bool) {
	switch v := item.(type) {
	case int64:
		template, is := IsIdsTemplateOnlyCommand(v)
		if template == "" {
			return false
		}
		//是唯一的一个模版的话就需要判断是否有角色引用，有引用就不能删；不是唯一的就随便删
		if is {
			list := GetTacacsRoleTemplateByCommandTemplate(template)
			if len(list) < 1 { //没有引用
				b = true
			} else { //有引用
				b = false
			}
		} else {
			b = true
		}
	case string:
		if IsTemplateOnlyCommand(v) {
			list := GetTacacsRoleTemplateByCommandTemplate(v)
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

func GetTemplateCmdInUsed() ([]string, []int64) {
	var templateCmdInUsed []string
	var templateIdCmdInUsed []int64
	inUsedRoles := GetInUsedRole()
	for _, inUsedRole := range inUsedRoles {
		inUsedRolesCmd := GetTacacsRoleTemplateByTemplate(inUsedRole).CommandTemplateList
		if !utils.IsValueInList(inUsedRolesCmd, templateCmdInUsed) {
			templateCmdInUsed = append(templateCmdInUsed, inUsedRolesCmd)
		}
	}

	tacacsCommandTemplates, _ := GetTacacsCommandTemplates()
	for _, tacacsCommandTemplate := range tacacsCommandTemplates {
		if utils.IsValueInList(tacacsCommandTemplate.Template, templateCmdInUsed) {
			templateIdCmdInUsed = append(templateIdCmdInUsed, tacacsCommandTemplate.ID)
		}

	}

	return templateCmdInUsed, templateIdCmdInUsed
}

func IsTemplateCmdInUsed(item interface{}) (b bool) {
	nameInUsed, idInUsed := GetTemplateCmdInUsed()
	switch v := item.(type) {
	case int64:
		b = utils.IsValueInList(v, idInUsed)
	case string:
		b = utils.IsValueInList(v, nameInUsed)
	}

	return
}

func GetTacacsTemplateCommandData2Md5() (string, error) {
	selectSql := "SELECT CONCAT(id, template, command_template) AS data_string FROM tacacs_command_template"
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
