package feishu

import (
	"fmt"
	"strings"
	"tacacs/pkg/public/db"
	"time"
)

// 飞书卡片 schema 2.0 builder。所有 builder 返回 map[string]any，调用方
// SendCardToPersonByUserId 里再 sonic.Marshal 成 content。
//
// header.template 的颜色对应业务语义：blue=待处理/进行中，green=成功/通过，
// red=失败/驳回，grey=关闭/撤回，orange=警告。
//
// 按钮 value 都用 map[string]any（即 JSON 对象），WS 回调侧 sonic.Unmarshal
// 后按字段取——后续加新按钮 / 加 reason 字段不需要重写解析。

const cardTimeLayout = "2006-01-02 15:04:05"

// baseCard 把 schema/config/header/elements 拼成一张完整卡片。caller 准备好
// elements 列表后扔进来就行——templates、按钮、文本通通由 elements 决定。
func baseCard(headerTitle, headerTemplate string, elements []any) map[string]any {
	if headerTemplate == "" {
		headerTemplate = "blue"
	}
	return map[string]any{
		"schema": "2.0",
		"config": map[string]any{
			"update_multi": true,
		},
		"header": map[string]any{
			"title": map[string]any{
				"tag":     "plain_text",
				"content": headerTitle,
			},
			"template": headerTemplate,
		},
		"body": map[string]any{
			"elements": elements,
		},
	}
}

func mdElement(content string) map[string]any {
	return map[string]any{
		"tag":     "markdown",
		"content": content,
	}
}

func approvalDetailMd(a *db.TacacsApproval) string {
	return fmt.Sprintf(
		"**申请人**：%s\n**申请角色**：%s\n**生效时间**：%s\n**到期时间**：%s\n**工单 ID**：%d",
		a.User,
		a.ApprovalPermissions,
		a.StartTime.Format(cardTimeLayout),
		a.EndTime.Format(cardTimeLayout),
		a.ID,
	)
}

// BuildAdminApprovalCard 给 admin 看的待审批卡片。带「通过 / 驳回」按钮，
// 按钮 value 里携带 action+approval_id，WS 回调侧用此触发 ApproveWithLock。
//
// schema 2.0 不再支持 `tag:"action"` 容器，按钮直接作为 elements 顶层；
// 横排两个按钮通过 column_set 实现，每个按钮的回调数据放进 behaviors 里。
func BuildAdminApprovalCard(a *db.TacacsApproval) map[string]any {
	body := approvalDetailMd(a) + "\n\n**当前状态**：等待审批"
	approveBtn := map[string]any{
		"tag":  "button",
		"text": map[string]any{"tag": "plain_text", "content": "通过"},
		"type": "primary",
		"size": "medium",
		"behaviors": []any{
			map[string]any{
				"type": "callback",
				"value": map[string]any{
					"action":      "approve",
					"approval_id": a.ID,
				},
			},
		},
	}
	rejectBtn := map[string]any{
		"tag":  "button",
		"text": map[string]any{"tag": "plain_text", "content": "驳回"},
		"type": "danger",
		"size": "medium",
		"behaviors": []any{
			map[string]any{
				"type": "callback",
				"value": map[string]any{
					"action":      "reject",
					"approval_id": a.ID,
				},
			},
		},
	}
	buttonRow := map[string]any{
		"tag":       "column_set",
		"flex_mode": "flow",
		"columns": []any{
			map[string]any{
				"tag":      "column",
				"width":    "auto",
				"weight":   1,
				"elements": []any{approveBtn},
			},
			map[string]any{
				"tag":      "column",
				"width":    "auto",
				"weight":   1,
				"elements": []any{rejectBtn},
			},
		},
	}
	return baseCard("权限申请待审批", "blue", []any{
		mdElement(body),
		buttonRow,
	})
}

// BuildApprovalResultCard 申请结果卡片（无按钮）。kind ∈ {approved,rejected,withdrawn,timeout}。
// operator 在 approved/rejected 时是审批的 admin，withdrawn 时是申请人本人，timeout 时填 "system"。
func BuildApprovalResultCard(a *db.TacacsApproval, kind string, operator string) map[string]any {
	var (
		title    string
		template string
		verb     string
	)
	switch kind {
	case "approved":
		title, template, verb = "权限申请已通过", "green", "已通过"
	case "rejected":
		title, template, verb = "权限申请已驳回", "red", "已驳回"
	case "withdrawn":
		title, template, verb = "权限申请已撤回", "grey", "已撤回"
	case "timeout":
		title, template, verb = "权限申请超时关闭", "grey", "超时关闭"
	default:
		title, template, verb = "权限申请通知", "blue", kind
	}
	when := time.Now().Format(cardTimeLayout)
	body := approvalDetailMd(a) + fmt.Sprintf("\n\n**当前状态**：%s by %s 于 %s", verb, operator, when)
	return baseCard(title, template, []any{mdElement(body)})
}

// BuildApprovalLifecycleCard approvalSystem 守护任务通知申请人。
// kind ∈ {timeout, expired}。
func BuildApprovalLifecycleCard(a *db.TacacsApproval, kind string, daysLeft int) map[string]any {
	var (
		title    string
		template string
		body     string
	)
	switch kind {
	case "timeout":
		title, template = "权限申请超时关闭", "grey"
		body = approvalDetailMd(a) + "\n\n**当前状态**：超过 24 小时未审批，流程自动关闭"
	case "expired":
		title, template = "权限已到期回收", "grey"
		body = approvalDetailMd(a) + "\n\n**当前状态**：已到期，权限已回收"
	default:
		title, template = "权限申请通知", "blue"
		body = approvalDetailMd(a)
	}
	return baseCard(title, template, []any{mdElement(body)})
}

// BuildRoleExpiryCard permissionSystem 的角色权限即将到期通知。
// tier ∈ {3d, 1d, 12h}，对应紧迫程度（橙→红）。expireAt 为合并区间的 EndTime。
func BuildRoleExpiryCard(user, level, tier string, expireAt time.Time) map[string]any {
	var (
		title    string
		template string
		urgency  string
	)
	switch tier {
	case "3d":
		title, template, urgency = "角色权限即将到期（3 天内）", "orange", "3 天内"
	case "1d":
		title, template, urgency = "角色权限即将到期（24 小时内）", "red", "24 小时内"
	case "12h":
		title, template, urgency = "角色权限即将到期（12 小时内）", "red", "12 小时内"
	default:
		title, template, urgency = "角色权限即将到期", "orange", "即将"
	}
	body := fmt.Sprintf(
		"**TACACS 用户**：%s\n**角色**：%s\n**到期时间**：%s\n\n该角色将在 %s 失效，如需继续使用请及时申请续期。",
		user, level, expireAt.Format("2006-01-02 15:04:05"), urgency,
	)
	return baseCard(title, template, []any{mdElement(body)})
}

// BuildPasswordCard permissionSystem 的密码相关通知。
// kind ∈ {warning, expired, restored}；warning 用 days/maxDays，expired 仅用 maxDays，
// restored 两个都不用。
func BuildPasswordCard(user, kind string, days, maxDays int) map[string]any {
	var (
		title    string
		template string
		body     string
	)
	switch kind {
	case "warning":
		title, template = "密码即将到期", "orange"
		body = fmt.Sprintf(
			"**TACACS 用户**：%s\n**密码使用天数**：%d 天\n**最大允许天数**：%d 天\n\n超过 %d 天后账号权限将被回收，请及时修改密码。",
			user, days, maxDays, maxDays,
		)
	case "expired":
		title, template = "密码已到期，权限回收", "red"
		body = fmt.Sprintf(
			"**TACACS 用户**：%s\n**密码使用天数**：超过 %d 天\n\n账号权限已回收，请及时修改密码后自动恢复。",
			user, maxDays,
		)
	case "restored":
		title, template = "密码已更新，权限恢复", "green"
		body = fmt.Sprintf("**TACACS 用户**：%s\n\n密码已更新，账号权限已恢复。", user)
	default:
		title, template = "密码通知", "blue"
		body = fmt.Sprintf("**TACACS 用户**：%s", user)
	}
	return baseCard(title, template, []any{mdElement(body)})
}

// BuildOnDutyChangeCard 值班人员变动通知（发给 admin）。
func BuildOnDutyChangeCard(addUsers, delUsers, currentDuty []string) map[string]any {
	var lines []string
	if len(delUsers) > 0 {
		lines = append(lines, fmt.Sprintf("**下线值班人员**：%s", strings.Join(delUsers, ", ")))
	}
	if len(addUsers) > 0 {
		lines = append(lines, fmt.Sprintf("**上线值班人员**：%s", strings.Join(addUsers, ", ")))
	}
	lines = append(lines, fmt.Sprintf("**当前值班人员（拥有操作权限）**：%s", strings.Join(currentDuty, ", ")))
	return baseCard("TACACS 值班人员变动", "blue", []any{mdElement(strings.Join(lines, "\n"))})
}

// BuildSystemAlertCard 通用系统告警卡片（发给运维）。title 概括类别（如 "日志上传失败"），
// body 用 markdown 写明细。template 默认 red，调用方可不传。
func BuildSystemAlertCard(title, body, template string) map[string]any {
	if title == "" {
		title = "TACACS 系统告警"
	}
	if template == "" {
		template = "red"
	}
	return baseCard(title, template, []any{mdElement(body)})
}
