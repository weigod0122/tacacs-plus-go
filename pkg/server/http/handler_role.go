package http

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"tacacs/pkg/public/db"
	"tacacs/pkg/public/utils"
	"tacacs/pkg/public/waitGroup"
	"tacacs/pkg/server/approvalSystem"

	"github.com/gin-gonic/gin"
)

func httpApiRoleGet(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()
	roleTemplate := db.GetTacacsRoleTemplate()
	resp := struct {
		Code int                     `json:"code"`
		Data []db.TacacsRoleTemplate `json:"data"`
	}{
		Code: 200,
		Data: roleTemplate,
	}
	c.JSON(http.StatusOK, resp)
}

func httpApiRoleCreate(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()
	type tempRoleAdd struct {
		Template            string `json:"template"`
		ServerTemplateList  string `json:"server_template_list"`
		CommandTemplateList string `json:"command_template_list"`
	}
	var req tempRoleAdd
	err := c.ShouldBindJSON(&req)
	b, _ := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("body(%v) convert to struct err: %v", strings.ReplaceAll(string(b), "\n", ""), err),
		})
		return
	}
	// 角色名 / 服务器模板名 / 命令模板名 都不允许包含英文逗号:
	// server_template_list、command_template_list 以及历史调用方对角色名的拼接
	// 都使用 "," 作为分隔符,值中混入逗号会破坏后续 Split 解析,导致权限错位。
	if strings.Contains(req.Template, ",") {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("Template(%v) must not contain comma ','", req.Template),
		})
		return
	}
	if temp := db.GetTacacsRoleTemplateByTemplate(req.Template); temp.Template != "" {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("Template(%v) already exists", req.Template),
		})
		return
	} else {
		for _, sT := range strings.Split(req.ServerTemplateList, ",") {
			serverTemplate, err := db.GetTacacsServerTemplatesByTemplate(sT)
			if err != nil || len(serverTemplate) < 1 {
				c.JSON(http.StatusMethodNotAllowed, gin.H{
					"code":    http.StatusMethodNotAllowed,
					"message": fmt.Sprintf("server_template_list(%v) does not exist, please create server_template_list first", req.ServerTemplateList),
				})
				return
			}
		}
		for _, cT := range strings.Split(req.CommandTemplateList, ",") {
			commandTemplate, err := db.GetTacacsCommandTemplatesByTemplate(cT)
			if err != nil || len(commandTemplate) < 1 {
				c.JSON(http.StatusMethodNotAllowed, gin.H{
					"code":    http.StatusMethodNotAllowed,
					"message": fmt.Sprintf("command_template_list(%v) does not exist, please create command_template_list first", req.CommandTemplateList),
				})
				return
			}
		}
		err = db.CreateRole(req.Template, req.ServerTemplateList, req.CommandTemplateList)
		if err != nil {
			c.JSON(http.StatusMethodNotAllowed, gin.H{
				"code":    http.StatusMethodNotAllowed,
				"message": fmt.Sprintf("Create role(%v) failed, err:%v", req, err),
			})
			return
		} else {
			c.JSON(http.StatusOK, gin.H{
				"code":    http.StatusOK,
				"massage": fmt.Sprintf("%v create success", req),
			})
			return
		}
	}

}

func httpApiRoleDelete(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()
	type tempRoleAdd struct {
		Template string `json:"template"`
	}
	var req tempRoleAdd
	err := c.ShouldBindJSON(&req)
	b, _ := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("body(%v) convert to struct err: %v", strings.ReplaceAll(string(b), "\n", ""), err),
		})
		return
	}
	temp := db.GetTacacsRoleTemplateByTemplate(req.Template)
	if temp.Template == "" {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("Template(%v) does not exist", req.Template),
		})
		return
	}

	//获取正在使用的角色，审批中和审批通过的角色为正在使用中
	approvingLists, _ := db.GetTacacsApproval(approvalSystem.ApprovalUnderReview)
	approvalPassedLists, _ := db.GetTacacsApproval(approvalSystem.ApprovalPass)
	var isUsingRole []string
	for _, approval := range append(approvingLists, approvalPassedLists...) {
		if !utils.IsValueInList(approval.ApprovalPermissions, isUsingRole) {
			isUsingRole = append(isUsingRole, approval.ApprovalPermissions)
		}
	}

	if utils.IsValueInList(req.Template, isUsingRole) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("Template(%v) is using", req.Template),
		})
		return
	} else {
		err := db.DeleteRole(req.Template)
		if err != nil {
			c.JSON(http.StatusMethodNotAllowed, gin.H{
				"code":    http.StatusMethodNotAllowed,
				"message": fmt.Sprintf("Template(%v) does not exist", req.Template),
			})
			return
		} else {
			c.JSON(http.StatusOK, gin.H{
				"code":    http.StatusOK,
				"massage": fmt.Sprintf("Template(%v) delete success", req.Template),
			})
			return
		}
	}

}
