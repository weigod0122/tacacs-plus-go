package http

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"tacacs/pkg/public/db"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/notify/feishu"
	"tacacs/pkg/public/utils"
	"tacacs/pkg/public/waitGroup"
	"tacacs/pkg/server/approvalSystem"
	"time"

	"github.com/gin-gonic/gin"
)

func httpApiApprovalCreate(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()

	type mission struct {
		User       string `json:"user"`
		Permission string `json:"permission"`
		Validity   string `json:"validity"`
		StartTime  string `json:"startTime"`
		EndTime    string `json:"endTime"`
	}
	var req mission
	err := c.ShouldBindJSON(&req)
	bodyBytes, _ := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    http.StatusForbidden,
			"message": fmt.Sprintf("body(%v) convert to struct err: %v", strings.ReplaceAll(string(bodyBytes), "\n", ""), err),
		})
		return
	}

	tacacsUsers := db.GetTacacsUser()
	if !utils.IsValueInList(req.User, tacacsUsers) {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    http.StatusForbidden,
			"message": fmt.Sprintf("user(%v) is not one of %v", req.User, tacacsUsers),
		})
		return
	}

	roleTemplates := db.GetRoles()

	if !utils.IsValueInList(req.Permission, roleTemplates) {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    http.StatusForbidden,
			"message": fmt.Sprintf("permission(%v) is not one of %v", req.Permission, roleTemplates),
		})
		return
	}

	useValidity := req.Validity != ""
	useTimeRange := req.StartTime != "" || req.EndTime != ""
	if useValidity && useTimeRange {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    http.StatusForbidden,
			"message": "有效期天数(validity)与开始/结束时间(startTime/endTime)只能选其一，不可同时填写",
		})
		return
	}
	if !useValidity && !useTimeRange {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    http.StatusForbidden,
			"message": "请填写有效期天数(validity)或开始/结束时间(startTime/endTime)，二者必须选其一",
		})
		return
	}

	var StartTime, EndTime time.Time
	if useValidity {
		validityNum, err := strconv.Atoi(req.Validity)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"message": fmt.Sprintf("输入的有效期‘%v’不是一个数字，请重新输入", req.Validity),
			})
			return
		}
		if validityNum > 365 || validityNum < 1 {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"message": fmt.Sprintf("有效期申请范围为1-365天，输入的有效期‘%v’不在区间内，请重新输入", req.Validity),
			})
			return
		}

		if strings.Contains(req.Permission, "permitAny") && validityNum > 1 {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"message": fmt.Sprintf("操作权限最大只支持申请%v天，请重新输入", req.Validity),
			})
			return
		}

		StartTime = time.Now()
		EndTime = StartTime.Add(time.Duration(validityNum) * 24 * time.Hour)
	} else {
		if req.StartTime == "" || req.EndTime == "" {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"message": "使用时间区间申请时，开始时间(startTime)和结束时间(endTime)必须同时填写",
			})
			return
		}

		const layout = "2006-01-02 15:04:05"
		StartTime, err = time.ParseInLocation(layout, req.StartTime, time.Local)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"message": fmt.Sprintf("开始时间‘%v’格式错误，请使用‘%v’格式", req.StartTime, layout),
			})
			return
		}
		EndTime, err = time.ParseInLocation(layout, req.EndTime, time.Local)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"message": fmt.Sprintf("结束时间‘%v’格式错误，请使用‘%v’格式", req.EndTime, layout),
			})
			return
		}
		if !EndTime.After(StartTime) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"message": fmt.Sprintf("结束时间‘%v’必须晚于开始时间‘%v’", req.EndTime, req.StartTime),
			})
			return
		}
		if !EndTime.After(time.Now()) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"message": fmt.Sprintf("结束时间‘%v’必须晚于当前时间，否则申请无意义", req.EndTime),
			})
			return
		}
		duration := EndTime.Sub(StartTime)
		if duration > 365*24*time.Hour {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"message": fmt.Sprintf("有效期申请范围为1-365天，当前申请时长%v小时已超出上限", int(duration.Hours())),
			})
			return
		}
		if strings.Contains(req.Permission, "permitAny") && duration > 24*time.Hour {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"message": "操作权限最大只支持申请1天，请重新输入",
			})
			return
		}
	}

	t := db.TacacsApproval{
		CreateTime:          time.Now(),
		User:                req.User,
		ApprovalPermissions: req.Permission,
		StartTime:           StartTime,
		EndTime:             EndTime,
		Status:              3,
	}
	approvalID, err := db.CreateTacacsApproval(t)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    http.StatusNotFound,
			"message": fmt.Sprintf("Create Approval err: %v", err),
		})
		return
	}
	t.ID = approvalID
	go func() {
		if err := feishu.SendCardToPersonByTacacsUserName(db.GetTacacsAdminUser(), feishu.BuildAdminApprovalCard(&t)); err != nil {
			log.Logger.Errorf("send admin approval card to %v err: %v", strings.Join(db.GetTacacsAdminUser(), ","), err)
		}
	}()
	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"massage": fmt.Sprintf("%v create success", req),
	})
}

func httpApiApprovalGet(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()

	data, err := db.GetTacacsApproval(approvalSystem.ApprovalAll)
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("Get Approval err: %v", err),
		})
		return
	}

	sort.Slice(data, func(i, j int) bool {
		return data[i].CreateTime.After(data[j].CreateTime)
	})

	resp := struct {
		Code int                  `json:"code"`
		Data []*db.TacacsApproval `json:"data"`
	}{
		Code: 200,
		Data: data,
	}
	c.JSON(http.StatusOK, resp)
}

func httpApiApprovalUpdate(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()

	type mission struct {
		Id     int64 `json:"id"`
		Status int   `json:"status"`
	}
	var req mission
	err := c.ShouldBindJSON(&req)
	bodyBytes, _ := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    http.StatusMethodNotAllowed,
			"message": fmt.Sprintf("body(%v) convert to struct err: %v", strings.ReplaceAll(string(bodyBytes), "\n", ""), err),
		})
		return
	}

	if req.Status != 4 && req.Status != 2 && req.Status != 0 {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("AuthenStatus(%v) is not one of [4, 2, 0]", req.Status),
		})
		return
	}

	// 操作者从 SwM 注入的可信头取（PR1 中间件验签后才会到这里）。撤回时该头是申请
	// 人本人；通过/拒绝时是 admin。空值兜底成 "system"，避免数据库写 NULL。
	operator := c.GetHeader("X-SwM-User")
	if operator == "" {
		operator = "system"
	}

	// 用乐观锁更新（仅 status=3 才会改写）。RowsAffected=0 时表示工单不存在、
	// 已被另一审批者抢先处理或已超时关闭，统一返回原来的兼容错误信息。
	rows, err := db.ApproveWithLock(req.Id, req.Status, operator)
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("Update is failed, err: %v", err),
		})
		return
	}
	if rows == 0 {
		c.JSON(http.StatusFailedDependency, gin.H{
			"code":    http.StatusFailedDependency,
			"message": fmt.Sprintf("Mission(id:%v) not found or status is not 3", req.Id),
		})
		return
	}

	// 通知申请人
	go func() {
		mission, _ := db.GetTacacsApprovalByID(req.Id)
		if mission != nil {
			kind := ""
			switch req.Status {
			case 4:
				kind = "approved"
			case 2:
				kind = "rejected"
			case 0:
				kind = "withdrawn"
			}
			if kind != "" {
				if err := feishu.SendCardToPersonByTacacsUserName([]string{mission.User}, feishu.BuildApprovalResultCard(mission, kind, operator)); err != nil {
					log.Logger.Errorf("send result card to %v err: %v", mission.User, err)
				}
			}
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": fmt.Sprintf("Update success, id:%v`s status change to %v", req.Id, req.Status),
	})
}
