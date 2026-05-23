package http

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"tacacs/pkg/public/db"
	"tacacs/pkg/public/notify/feishu"
	"tacacs/pkg/public/utils"
	"tacacs/pkg/public/waitGroup"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	updatePasswordErrUser map[string]int8
	checkPasswordErrUser  map[string]int8
)

func httpApiUserGet(c *gin.Context) {
	statusMap := map[string]string{
		"0": "已停用",
		"1": "使用中",
		"2": "暂停使用",
	}
	type info struct {
		User               string
		PhoneNumber        string `db:"phone_number"`
		Email              string
		CreateTime         string
		Role               string
		RoleUpdateTime     string
		PasswordUpdateTime string `db:"password_update_time"`
		Status             string `db:"status"`
		StatusUpdateTime   string `db:"status_update_time"`
		Notes              string `db:"notes"`
	}

	var respBody []info

	users, err := db.GetTacacsUserInfos()
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("get tacacs user info err: %v", err),
		})
		return
	}
	for _, user := range users {
		i := info{
			User:               user.User,
			PhoneNumber:        user.PhoneNumber,
			Email:              user.Email,
			CreateTime:         user.CreateTime,
			Role:               user.Role,
			RoleUpdateTime:     user.RoleUpdateTime,
			PasswordUpdateTime: user.PasswordUpdateTime,
			Status:             statusMap[user.Status],
			StatusUpdateTime:   user.StatusUpdateTime,
			Notes:              user.Notes,
		}
		respBody = append(respBody, i)
	}

	resp := struct {
		Code int    `json:"code"`
		Data []info `json:"data"`
	}{
		Code: 200,
		Data: respBody,
	}
	c.JSON(http.StatusOK, resp)

}

func httpApiUserGetAdmin(c *gin.Context) {
	c.JSON(http.StatusOK, db.GetTacacsAdminUser())
}

func httpApiUserCreate(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()

	type tacacsUser struct {
		User        string `json:"user"`
		PhoneNumber string `json:"phone_number"`
		Email       string `json:"email"`
		Password    string `json:"password"`
		Notes       string `json:"notes"`
	}

	var req tacacsUser
	err := c.ShouldBindJSON(&req)
	bodyBytes, _ := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("body(%v) convert to struct err: %v", strings.ReplaceAll(string(bodyBytes), "\n", ""), err),
		})
		return
	}

	users, err := db.GetTacacsUserInfos()
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("GetTacacsUserInfos err: %v", err),
		})
		return
	}
	if req.User == "" || req.PhoneNumber == "" || req.Email == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": fmt.Sprintf("传入的参数存在缺失: %v", err),
		})
		return
	}

	_, getFeishuUserIdErr := feishu.GetUserIdByBasicInfo(req.Email, req.PhoneNumber)
	if getFeishuUserIdErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": fmt.Sprintf("通过邮箱和手机号查询飞书用户id失败：%v", getFeishuUserIdErr),
		})
		return
	}

	userList := make(map[string]bool)
	var code int
	var message string
	for _, u := range users {
		userList[u.User] = true
	}
	if userList[req.User] == false {
		err = db.CreateUser(req.User, req.PhoneNumber, req.Email, req.Password, req.Notes)
	} else {
		err = fmt.Errorf("用户已存在，不可重复创建")
	}

	if err != nil {
		code = http.StatusFailedDependency
		message = fmt.Sprintf("%v create failed, because:%v", req.User, err)
	} else {
		code = http.StatusOK
		message = fmt.Sprintf("%v create success", req.User)
	}
	c.JSON(code, gin.H{
		"code":    code,
		"message": message,
	})
	return
}

func resetUserPassword(user, password string) error {
	err1 := db.UpdateUserStatus(user, "1")
	err2 := db.UpdateUserPassword(user, password)
	if err1 != nil || err2 != nil {
		return fmt.Errorf("UpdateUserStatus: %v; UpdateUserPassword: %v", err1, err2)
	}
	return nil
}

func httpApiUserResetPassword(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()

	type tacacsUser struct {
		Operator         string `json:"operator"`
		OperatorPassword string `json:"operator_password"`
		User             string `json:"user"`
		Password         string `json:"password"`
	}

	var req tacacsUser
	err := c.ShouldBindJSON(&req)
	bodyBytes, _ := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("body(%v) convert to struct err: %v", strings.ReplaceAll(string(bodyBytes), "\n", ""), err),
		})
		return
	}

	if req.Operator == "" || req.OperatorPassword == "" || req.User == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "传入的参数存在缺失（operator/operatorPassword/user/password 均不能为空）",
		})
		return
	}

	if req.Operator == req.User {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    http.StatusForbidden,
			"message": "不允许重置当前登录用户自己的密码",
		})
		return
	}

	users, err := db.GetTacacsUserInfos()
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("GetTacacsUserInfos err: %v", err),
		})
		return
	}

	var operatorExist, operatorPwdOk, userExist bool
	for _, u := range users {
		if u.User == req.Operator {
			operatorExist = true
			operatorPwdOk = utils.CheckPasswordHash(req.OperatorPassword, u.Password)
		}
		if u.User == req.User {
			userExist = true
		}
	}

	if !operatorExist || !operatorPwdOk {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    http.StatusUnauthorized,
			"message": "登录用户名或密码错误",
		})
		return
	}

	if !utils.IsValueInList(req.Operator, db.GetTacacsAdminUser()) {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    http.StatusForbidden,
			"message": fmt.Sprintf("用户(%v)非管理员，无权重置密码", req.Operator),
		})
		return
	}

	if !userExist {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("用户(%v)不存在，无法重置密码", req.User),
		})
		return
	}

	var code int
	var message string
	if err = resetUserPassword(req.User, req.Password); err != nil {
		code = http.StatusFailedDependency
		message = fmt.Sprintf("%v reset password failed, because:%v", req.User, err)
	} else {
		code = http.StatusOK
		message = fmt.Sprintf("%v reset password success by operator %v", req.User, req.Operator)
	}
	c.JSON(code, gin.H{
		"code":    code,
		"message": message,
	})
	return
}

func httpApiUserUpdatePassword(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()
	type tacacsUser struct {
		User        string `json:"user"`
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}

	var req tacacsUser
	err := c.ShouldBindJSON(&req)
	bodyBytes, _ := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("body(%v) convert to struct err: %v", strings.ReplaceAll(string(bodyBytes), "\n", ""), err),
		})
		return
	}

	if updatePasswordErrUser[req.User] > 3 {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("用户(%v)一小时内密码输入错误次数超过3次，请求已拒绝，请一小时后再试", req.User),
		})
		return
	}

	var isExist bool
	var isOldPasswordPass bool
	tacacsUserLists, _ := db.GetTacacsUserInfos()
	for _, tacacsUserInfo := range tacacsUserLists {
		if tacacsUserInfo.User == req.User {
			isExist = true
			isOldPasswordPass = utils.CheckPasswordHash(req.OldPassword, tacacsUserInfo.Password)
			break
		}
	}

	if !isExist {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("用户(%v)不存在", req.User),
		})
		return
	}

	if !isOldPasswordPass {
		updatePasswordErrUser[req.User]++
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": "原密码校验不通过，请重试",
		})
		return
	}

	var code int
	var message string
	err = db.UpdateUserPassword(req.User, req.NewPassword)
	if err != nil {
		code = http.StatusFailedDependency
		message = fmt.Sprintf("%v update password failed, because:%v", req.User, err)
	} else {
		code = http.StatusOK
		message = fmt.Sprintf("%v update password success", req.User)
	}
	updatePasswordErrUser[req.User] = 0
	c.JSON(code, gin.H{
		"code":    code,
		"message": message,
	})
	return
}

func httpApiUserUpdateNotes(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()

	type tacacsUser struct {
		User  string `json:"user"`
		Notes string `json:"notes"`
	}

	var req tacacsUser
	err := c.ShouldBindJSON(&req)
	bodyBytes, _ := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("body(%v) convert to struct err: %v", strings.ReplaceAll(string(bodyBytes), "\n", ""), err),
		})
		return
	}
	var code int
	var message string
	err = db.UpdateUserNotes(req.User, req.Notes)
	if err != nil {
		code = http.StatusFailedDependency
		message = fmt.Sprintf("%v update notes failed, because:%v", req.User, err)
	} else {
		code = http.StatusOK
		message = fmt.Sprintf("%v update notes success", req.User)
	}
	c.JSON(code, gin.H{
		"code":    code,
		"message": message,
	})
	return
}

func httpApiUserUpdateBasicInfo(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()

	type tacacsUser struct {
		User        string `json:"user"`
		Email       string `json:"email"`
		PhoneNumber string `json:"phone_number"`
	}

	var req tacacsUser
	err := c.ShouldBindJSON(&req)
	bodyBytes, _ := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("body(%v) convert to struct err: %v", strings.ReplaceAll(string(bodyBytes), "\n", ""), err),
		})
		return
	}
	_, getFeishuUserIdErr := feishu.GetUserIdByBasicInfo(req.Email, req.PhoneNumber)
	if getFeishuUserIdErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": fmt.Sprintf("通过邮箱和手机号查询飞书用户id失败：%v", getFeishuUserIdErr),
		})
		return
	}
	var code int
	var message string
	err = db.UpdateUserEmailAndPhoneNumber(req.User, req.Email, req.PhoneNumber)
	if err != nil {
		code = http.StatusFailedDependency
		message = fmt.Sprintf("%v update email failed, because:%v", req.User, err)
	} else {
		code = http.StatusOK
		message = fmt.Sprintf("%v update email success", req.User)
	}
	c.JSON(code, gin.H{
		"code":    code,
		"message": message,
	})
	return
}

func httpApiUserDelete(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()

	type tacacsUser struct {
		User string `json:"user"`
	}

	var req tacacsUser
	err := c.ShouldBindJSON(&req)
	bodyBytes, _ := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("body(%v) convert to struct err: %v", strings.ReplaceAll(string(bodyBytes), "\n", ""), err),
		})
		return
	}
	var code int
	var message string
	err = db.UpdateUserStatus(req.User, "0")
	if err != nil {
		code = http.StatusFailedDependency
		message = fmt.Sprintf("delete user:%v failed, because:%v", req.User, err)
	} else {
		code = http.StatusOK
		message = fmt.Sprintf("delete user:%v success", req.User)
	}
	c.JSON(code, gin.H{
		"code":    code,
		"message": message,
	})
	return
}

func updatePasswordErrUserUpdate() {
	for {
		updatePasswordErrUser = make(map[string]int8)
		time.Sleep(time.Hour)
	}
}

func checkPasswordErrUserUpdate() {
	for {
		checkPasswordErrUser = make(map[string]int8)
		time.Sleep(time.Hour)
	}
}

func httpApiCheckUser(c *gin.Context) {
	type user struct {
		User     string `json:"user"`
		Password string `json:"password"`
	}
	var req user
	err := c.ShouldBindJSON(&req)
	bodyBytes, _ := io.ReadAll(c.Request.Body)
	if err != nil {
		c.String(http.StatusMethodNotAllowed, "body(%v)格式错误：%v", strings.ReplaceAll(string(bodyBytes), "\n", ""), err)
		return
	}

	if checkPasswordErrUser[req.User] > 3 {
		c.String(http.StatusMethodNotAllowed, "用户(%v)一小时内密码输入错误次数超过3次，请求已拒绝，请一小时后再试", req.User)
		return
	}

	var isExist bool
	var isPasswordPass bool
	tacacsUserLists, _ := db.GetTacacsUserInfos()
	for _, tacacsUserInfo := range tacacsUserLists {
		if tacacsUserInfo.User == req.User {
			isExist = true
			isPasswordPass = utils.CheckPasswordHash(req.Password, tacacsUserInfo.Password)
			break
		}
	}

	if !isExist {
		c.String(http.StatusMethodNotAllowed, "用户(%v)不存在", req.User)
		return
	}

	if !isPasswordPass {
		checkPasswordErrUser[req.User]++
		c.String(http.StatusMethodNotAllowed, "密码校验不通过，请重试")
		return
	}
	checkPasswordErrUser[req.User] = 0
	c.String(http.StatusOK, "通过")
}

func httpClearCheckPasswordErrUser(c *gin.Context) {
	for k := range checkPasswordErrUser {
		checkPasswordErrUser[k] = 0
	}
	c.String(http.StatusOK, "完成")
}
func httpClearUpdatePasswordErrUser(c *gin.Context) {
	for k := range updatePasswordErrUser {
		updatePasswordErrUser[k] = 0
	}
	c.String(http.StatusOK, "完成")

}
