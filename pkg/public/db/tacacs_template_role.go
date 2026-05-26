package db

import (
	"strings"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/utils"
)

type TacacsRoleTemplate struct {
	ID                  int64  `db:"id"`
	Template            string `db:"template"`
	ServerTemplateList  string `db:"server_template_list"`
	CommandTemplateList string `db:"command_template_list"`
}

// tacacsRoleTemplateCols 必须与下面所有 Scan 顺序一一对应。
// 不用 SELECT *,避免 DBA 加列后 Scan 数量不匹配直接报错。
const tacacsRoleTemplateCols = "id, template, server_template_list, command_template_list"

func GetTacacsRoleTemplate() []TacacsRoleTemplate {
	rows, err := DbRead.Query("SELECT " + tacacsRoleTemplateCols + " FROM tacacs_role_template")
	if err != nil {
		log.Logger.Errorf("select tacacs_role_template from database is failed, because %v", err)
		return nil
	}
	defer rows.Close()

	var templates []TacacsRoleTemplate
	for rows.Next() {
		var t TacacsRoleTemplate
		if err := rows.Scan(&t.ID, &t.Template, &t.ServerTemplateList, &t.CommandTemplateList); err != nil {
			log.Logger.Errorf("read database data is failed ,because %v", err)
			continue
		}
		templates = append(templates, t)
	}

	return templates
}
func GetTacacsRoleTemplates() ([]*TacacsRoleTemplate, error) {
	rows, err := DbRead.Query("SELECT " + tacacsRoleTemplateCols + " FROM tacacs_role_template")
	if err != nil {
		log.Logger.Errorf("select tacacs_role_template from database is failed, because %v", err)
		return nil, err
	}
	defer rows.Close()

	var templates []*TacacsRoleTemplate
	for rows.Next() {
		t := &TacacsRoleTemplate{}
		if err := rows.Scan(&t.ID, &t.Template, &t.ServerTemplateList, &t.CommandTemplateList); err != nil {
			log.Logger.Errorf("read database data is failed ,because %v", err)
			return nil, err
		}
		templates = append(templates, t)
	}

	return templates, nil
}

func GetTacacsRoleTemplateByTemplate(template string) TacacsRoleTemplate {
	var t TacacsRoleTemplate
	rows, err := DbRead.Query("SELECT "+tacacsRoleTemplateCols+" FROM tacacs_role_template WHERE template = ?", template)
	if err != nil {
		log.Logger.Errorf("select tacacs_role_template from database is failed, because %v", err)
		return t
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&t.ID, &t.Template, &t.ServerTemplateList, &t.CommandTemplateList); err != nil {
			log.Logger.Errorf("read database data is failed ,because %v", err)
		}
		break
	}

	return t
}

func GetTacacsRoleTemplateByServerTemplate(serverTemplate string) []TacacsRoleTemplate {
	var t []TacacsRoleTemplate
	rows, err := DbRead.Query("SELECT "+tacacsRoleTemplateCols+" FROM tacacs_role_template WHERE server_template_list = ?", serverTemplate)
	if err != nil {
		log.Logger.Errorf("select tacacs_role_template from database is failed, because %v", err)
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var i TacacsRoleTemplate
		if err = rows.Scan(&i.ID, &i.Template, &i.ServerTemplateList, &i.CommandTemplateList); err != nil {
			log.Logger.Errorf("read database data is failed ,because %v", err)
			continue
		}
		t = append(t, i)
	}
	return t
}

func GetTacacsRoleTemplateByCommandTemplate(commandTemplate string) []TacacsRoleTemplate {
	var t []TacacsRoleTemplate
	rows, err := DbRead.Query("SELECT "+tacacsRoleTemplateCols+" FROM tacacs_role_template WHERE command_template_list = ?", commandTemplate)
	if err != nil {
		log.Logger.Errorf("select tacacs_role_template from database is failed, because %v", err)
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var i TacacsRoleTemplate
		if err = rows.Scan(&i.ID, &i.Template, &i.ServerTemplateList, &i.CommandTemplateList); err != nil {
			log.Logger.Errorf("read database data is failed ,because %v", err)
			continue
		}
		t = append(t, i)
	}
	return t
}

func GetRoles() (roles []string) {
	for _, r := range GetTacacsRoleTemplate() {
		roles = append(roles, r.Template)
	}
	return
}

func CreateRole(template, serverTemplateList, commandTemplateList string) error {
	insertSql := "INSERT INTO tacacs_role_template(template, server_template_list, command_template_list) VALUES (?, ?, ?)"
	_, err := DbWrite.Exec(insertSql, template, serverTemplateList, commandTemplateList)
	if err != nil {
		log.Logger.Errorf("DB Exec err%v", err)
		return err
	}
	BumpMetaVersion(MetaKeyRole)
	return nil
}

func DeleteRole(template string) error {
	deleteSql := "DELETE FROM tacacs_role_template WHERE template = ?"
	_, err := DbWrite.Exec(deleteSql, template)
	if err != nil {
		log.Logger.Errorf("DB Exec err%v", err)
		return err
	}
	BumpMetaVersion(MetaKeyRole)
	return nil
}

func GetInUsedRole() (roles []string) {
	tacacsUserInfos, _ := GetTacacsUserInfos()
	for _, user := range tacacsUserInfos {
		if user.Status == "1" {
			rs := strings.Split(user.Role, ",")
			for _, r := range rs {
				if r != "null" && !utils.IsValueInList(r, roles) {
					roles = append(roles, r)
				}
			}

		}
	}
	return
}
