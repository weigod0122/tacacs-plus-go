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

func httpApiTemplateServerGet(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()
	serverTemplate, err := db.GetTacacsServerTemplates()
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("GetTacacsServerTemplates err: %v", err),
		})
		return
	}
	resp := struct {
		Code int                        `json:"code"`
		Data []*db.TacacsServerTemplate `json:"data"`
	}{
		Code: 200,
		Data: serverTemplate,
	}
	c.JSON(http.StatusOK, resp)
	return
}

func httpApiTemplateServerAdd(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()
	type tempSerAdd struct {
		Template       string   `json:"template"`
		TemplateDetail []string `json:"templateDetail"`
	}
	var req tempSerAdd
	err := c.ShouldBindJSON(&req)
	b, _ := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("body(%v) convert to struct err: %v", strings.ReplaceAll(string(b), "\n", ""), err),
		})
		return
	}
	if len(req.TemplateDetail) < 1 {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("body(%v) TemplateDetail is null", strings.ReplaceAll(string(b), "\n", "")),
		})
		return
	}

	if db.IsTemplateServerInUsed(req.Template) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": "Template is using!",
		})
		return
	}

	serverLists, err := db.GetTacacsServerTemplatesByTemplate(req.Template)
	if err != nil {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("GetTacacsServerTemplatesByTemplate failed, err: %v", err),
		})
		return
	}
	serverIsHad := make(map[string]bool)
	for _, t := range serverLists {
		serverIsHad[t.ServerTemplate] = true
		//serverIsHad = append(serverIsHad, t.ServerTemplate)
	}
	var serverNeedAdd []string
	for _, ip := range req.TemplateDetail {
		strType, _ := utils.GetNetworkType(ip)
		if strType != 1 && strType != 2 {
			c.JSON(http.StatusMethodNotAllowed, gin.H{
				"code":    http.StatusMethodNotAllowed,
				"message": fmt.Sprintf("The string '%v' is not in ip format", ip),
			})
			return
		}
		if serverIsHad[ip] != true {
			serverNeedAdd = append(serverNeedAdd, ip)
		}
	}

	if len(serverNeedAdd) < 1 {
		c.JSON(http.StatusOK, gin.H{
			"code":    http.StatusOK,
			"message": fmt.Sprintf("Servers(%v) has been added, do not add repeatedly ", req.TemplateDetail),
		})
		return
	}

	var code int
	var message string
	err = db.AddTacacsServerTemplates(req.Template, serverNeedAdd)
	if err != nil {
		code = http.StatusNotFound
		message = fmt.Sprintf("Template:%v,TemplateDetail:%v add failed, because:%v", req.Template, serverNeedAdd, err)
	} else {
		code = http.StatusOK
		message = fmt.Sprintf("Template:%v,TemplateDetail:%v add success", req.Template, serverNeedAdd)
	}
	c.JSON(code, gin.H{
		"code":    code,
		"message": message,
	})
	return

}

func httpApiTemplateServerDelete(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()
	type tempServerDel struct {
		Id       int64  `json:"id"`
		Template string `json:"template"`
	}
	var req tempServerDel
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
		if db.IsTemplateServerInUsed(req.Id) {
			c.JSON(http.StatusMethodNotAllowed, gin.H{
				"code":    http.StatusMethodNotAllowed,
				"message": "Template is using!",
			})
			return
		}
		if db.IsIdExistsInTacacsServerTemplate(req.Id) {
			if db.IsIdOrTemplateDeleteServer(req.Id) {
				err := db.DelTacacsServerTemplate(req.Id)
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
		if db.IsTemplateServerInUsed(req.Template) {
			c.JSON(http.StatusMethodNotAllowed, gin.H{
				"code":    http.StatusMethodNotAllowed,
				"message": "Template is using!",
			})
			return
		}
		if db.IsTemplateExistsInTacacsServerTemplate(req.Template) {
			if db.IsIdOrTemplateDeleteServer(req.Template) {
				err := db.DelTacacsServerTemplate(req.Template)
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
