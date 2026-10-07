package http

import (
	"net/http"
	"strings"
	"tacacs/pkg/public/db"

	"github.com/gin-gonic/gin"
)

// httpInternalUserStatus is consumed only by SwM's signed internal request.
// Keeping it outside /tacacs prevents a browser session from querying another
// user's state through the public reverse proxy, while allowing SwM to revoke
// sessions promptly after an administrator changes status.
func httpInternalUserStatus(c *gin.Context) {
	username := strings.TrimSpace(c.Param("user"))
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": "用户名不能为空"})
		return
	}
	user, err := db.GetTacacsUserInfoByUserName(username)
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": "查询用户状态失败",
		})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": http.StatusNotFound, "message": "用户不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "status": user.Status})
}
