package http

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"tacacs/pkg/public/db"
	"tacacs/pkg/public/utils"
	"tacacs/pkg/public/waitGroup"

	"github.com/gin-gonic/gin"
)

func httpApiTemplateCmdGet(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()
	cmdTempLists, err := db.GetTacacsCommandTemplates()
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("GetTacacsCommandTemplates err: %v", err),
		})
		return
	}
	resp := struct {
		Code int                         `json:"code"`
		Data []*db.TacacsCommandTemplate `json:"data"`
	}{
		Code: 200,
		Data: cmdTempLists,
	}
	c.JSON(http.StatusOK, resp)
}

func httpApiTemplateCmdAdd(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()
	type tempCmdAdd struct {
		Template       string   `json:"template"`
		TemplateDetail []string `json:"templateDetail"`
	}
	var req tempCmdAdd
	err := c.ShouldBindJSON(&req)
	b, _ := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("body(%v) convert to struct err: %v", strings.ReplaceAll(string(b), "\n", ""), err),
		})
		return
	}

	if db.IsTemplateCmdInUsed(req.Template) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": "Template is using!",
		})
		return
	}

	cmdLists, err := db.GetTacacsCommandTemplatesByTemplate(req.Template)
	if err != nil {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("GetTacacsCommandTemplatesByTemplate failed, err: %v", err),
		})
		return
	}
	var cmdIsHad []string
	for _, t := range cmdLists {
		cmdIsHad = append(cmdIsHad, t.CommandTemplate)
	}
	var cmdNeedAdd []string
	for _, cmd := range req.TemplateDetail {
		if !utils.IsValueInList(cmd, cmdIsHad) {
			cmdNeedAdd = append(cmdNeedAdd, cmd)
		}
	}
	if len(cmdNeedAdd) < 1 {
		c.JSON(http.StatusOK, gin.H{
			"code":    http.StatusOK,
			"message": fmt.Sprintf("Command(%v) has been added, do not add repeatedly ", req.TemplateDetail),
		})
		return
	}

	var code int
	var message string
	err = db.AddTacacsCommandTemplate(req.Template, cmdNeedAdd)
	if err != nil {
		code = http.StatusNotFound
		message = fmt.Sprintf("Template:%v,TemplateDetail:%v add failed, because:%v", req.Template, cmdNeedAdd, err)
	} else {
		code = http.StatusOK
		message = fmt.Sprintf("Template:%v,TemplateDetail:%v add success", req.Template, cmdNeedAdd)
	}
	c.JSON(code, gin.H{
		"code":    code,
		"message": message,
	})
	return
}

func httpApiTemplateCmdDelete(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()
	type tempCmdDel struct {
		Id       int64  `json:"id"`
		Template string `json:"template"`
	}
	var req tempCmdDel
	err := c.ShouldBindJSON(&req)
	b, _ := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("body(%v) convert to struct err: %v", strings.ReplaceAll(string(b), "\n", ""), err),
		})
		return
	}

	if (req.Id == 0 && req.Template == "") || (req.Id != 0 && req.Template != "") {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": "id and template cannot be empty or have values at the same time",
		})
		return
	}

	if req.Id != 0 {
		if db.IsTemplateCmdInUsed(req.Id) {
			c.JSON(http.StatusMethodNotAllowed, gin.H{
				"code":    http.StatusMethodNotAllowed,
				"message": "Template is using!",
			})
			return
		}
		if db.IsIdExistsInTacacsCommandTemplate(req.Id) {
			if db.IsIdOrTemplateDeleteCommand(req.Id) {
				err := db.DelTacacsCommandTemplate(req.Id)
				if err != nil {
					c.JSON(http.StatusMethodNotAllowed, gin.H{
						"code":    http.StatusMethodNotAllowed,
						"message": fmt.Sprintf("delete by id(%v) failed, err: %v", req.Id, err),
					})
					return
				} else {
					c.JSON(http.StatusOK, gin.H{
						"code":    http.StatusOK,
						"message": fmt.Sprintf("delete by id(%v) success", req.Id),
					})
					return
				}
			} else {
				c.JSON(http.StatusOK, gin.H{
					"code":    http.StatusOK,
					"message": fmt.Sprintf("id(%v) delete false. This id is the last entry in the template. The template is still being referenced. Please check the user role.", req.Id),
				})
				return
			}

		} else {
			c.JSON(http.StatusMethodNotAllowed, gin.H{
				"code":    http.StatusMethodNotAllowed,
				"message": fmt.Sprintf("id(%v) is not exists", req.Id),
			})
			return
		}
	}
	if req.Template != "" {
		if db.IsTemplateCmdInUsed(req.Template) {
			c.JSON(http.StatusMethodNotAllowed, gin.H{
				"code":    http.StatusMethodNotAllowed,
				"message": "Template is using!",
			})
			return
		}
		if db.IsTemplateExistsInTacacsCommandTemplate(req.Template) {
			if db.IsIdOrTemplateDeleteCommand(req.Template) {
				err := db.DelTacacsCommandTemplate(req.Template)
				if err != nil {
					c.JSON(http.StatusMethodNotAllowed, gin.H{
						"code":    http.StatusMethodNotAllowed,
						"message": fmt.Sprintf("delete by template(%v) failed, err: %v", req.Template, err),
					})
					return
				} else {
					c.JSON(http.StatusOK, gin.H{
						"code":    http.StatusOK,
						"message": fmt.Sprintf("delete by template(%v) success", req.Template),
					})
					return
				}
			} else {
				c.JSON(http.StatusOK, gin.H{
					"code":    http.StatusOK,
					"message": fmt.Sprintf("template(%v) delete false. The template is still being referenced. Please check the user role.", req.Template),
				})
				return
			}

		} else {
			c.JSON(http.StatusMethodNotAllowed, gin.H{
				"code":    http.StatusMethodNotAllowed,
				"message": fmt.Sprintf("template(%v) is not exists", req.Template),
			})
			return
		}
	}

}
