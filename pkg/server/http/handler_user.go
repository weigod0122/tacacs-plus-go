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
	// Failed-password counters are process-local lockout hints.  They are
	// deliberately bounded to a small integer, but still need a mutex because
	// login and password-change requests run concurrently with the hourly
	// cleanup goroutines.
	updatePasswordErrUser = make(map[string]int8)
	checkPasswordErrUser  = make(map[string]int8)
)

func httpApiUserGet(c *gin.Context) {
	username, isAdmin, verified := requireVerifiedIdentity(c)
	if !verified {
		c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "missing verified identity"})
		return
	}

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
		if !canReadOwnedRecord(username, user.User, isAdmin) {
			continue
		}
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
	if !requireAdminIdentity(c) {
		return
	}
	c.JSON(http.StatusOK, db.GetTacacsAdminUser())
}

func httpApiUserCreate(c *gin.Context) {
	if !requireAdminIdentity(c) {
		return
	}
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
	return db.ResetUserPassword(user, password)
}

func httpApiUserResetPassword(c *gin.Context) {
	// The proxy ACL is the first gate, but keep the privilege check at the
	// handler boundary as well so a directly signed Server request cannot turn
	// this recovery endpoint into a general password setter.
	if !requireAdminIdentity(c) {
		return
	}
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
	operator, _, verified := requireVerifiedIdentity(c)
	if !verified {
		c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "missing verified identity"})
		return
	}
	if req.Operator != operator {
		c.JSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "message": "operator must match signed identity"})
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
		// A successful administrative reset is an explicit recovery action. Do
		// not leave the target trapped behind the one-hour failed-login
		// lockout caused by attempts with the old password.
		resetCheckPasswordFailure(req.User)
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
	operator, isAdmin, verified := requireVerifiedIdentity(c)
	if !verified {
		c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "missing verified identity"})
		return
	}
	if !isAdmin && req.User != operator {
		c.JSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "message": "只能修改自己的密码"})
		return
	}

	if passwordUpdateFailureCount(req.User) > 3 {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("用户(%v)一小时内密码输入错误次数超过3次，请求已拒绝，请一小时后再试", req.User),
		})
		return
	}

	var isExist bool
	var userStatus string
	var isOldPasswordPass bool
	tacacsUserLists, _ := db.GetTacacsUserInfos()
	for _, tacacsUserInfo := range tacacsUserLists {
		if tacacsUserInfo.User == req.User {
			isExist = true
			userStatus = tacacsUserInfo.Status
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
	if userStatus == "0" {
		// Disabled accounts are recovered through the administrator reset flow;
		// self-service password changes must not reactivate one.
		c.JSON(http.StatusForbidden, gin.H{
			"code":    http.StatusForbidden,
			"message": "账号已停用，请联系管理员恢复",
		})
		return
	}

	if !isOldPasswordPass {
		incrementPasswordUpdateFailure(req.User)
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
	if code == http.StatusOK {
		resetPasswordUpdateFailure(req.User)
	}
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
	operator, isAdmin, verified := requireVerifiedIdentity(c)
	if !verified {
		c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "missing verified identity"})
		return
	}
	if !isAdmin && req.User != operator {
		c.JSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "message": "只能修改自己的备注"})
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
	operator, isAdmin, verified := requireVerifiedIdentity(c)
	if !verified {
		c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "missing verified identity"})
		return
	}
	if !isAdmin && req.User != operator {
		c.JSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "message": "只能修改自己的基础信息"})
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
	if !requireAdminIdentity(c) {
		return
	}
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
		clearPasswordUpdateFailures()
		time.Sleep(time.Hour)
	}
}

func checkPasswordErrUserUpdate() {
	for {
		clearCheckPasswordFailures()
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
	operator, isAdmin, verified := requireVerifiedIdentity(c)
	if !verified {
		c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "missing verified identity"})
		return
	}
	if !isAdmin && req.User != operator {
		c.JSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "message": "只能校验自己的密码"})
		return
	}

	if checkPasswordFailureCount(req.User) > 3 {
		c.String(http.StatusMethodNotAllowed, "用户(%v)一小时内密码输入错误次数超过3次，请求已拒绝，请一小时后再试", req.User)
		return
	}

	var isExist bool
	var isPasswordPass bool
	var userStatus string
	tacacsUserLists, _ := db.GetTacacsUserInfos()
	for _, tacacsUserInfo := range tacacsUserLists {
		if tacacsUserInfo.User == req.User {
			isExist = true
			userStatus = tacacsUserInfo.Status
			isPasswordPass = utils.CheckPasswordHash(req.Password, tacacsUserInfo.Password)
			break
		}
	}

	if !isExist {
		c.String(http.StatusMethodNotAllowed, "用户(%v)不存在", req.User)
		return
	}
	if userStatus != "1" && userStatus != "2" {
		// SwM translates this non-200 response to the generic login failure
		// page, so the disabled/unknown state is not disclosed to callers.
		c.JSON(http.StatusForbidden, gin.H{
			"code":    http.StatusForbidden,
			"message": "账号当前不可登录",
		})
		return
	}

	if !isPasswordPass {
		incrementCheckPasswordFailure(req.User)
		c.String(http.StatusMethodNotAllowed, "密码校验不通过，请重试")
		return
	}
	resetCheckPasswordFailure(req.User)
	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"status":  userStatus,
		"message": "通过",
	})
}

func httpClearCheckPasswordErrUser(c *gin.Context) {
	if !requireAdminIdentity(c) {
		return
	}
	clearCheckPasswordFailures()
	c.String(http.StatusOK, "完成")
}
func httpClearUpdatePasswordErrUser(c *gin.Context) {
	if !requireAdminIdentity(c) {
		return
	}
	clearPasswordUpdateFailures()
	c.String(http.StatusOK, "完成")

}
