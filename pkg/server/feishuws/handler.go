package feishuws

import (
	"context"
	"fmt"
	"tacacs/pkg/public/db"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/notify/feishu"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
)

// handleCardAction 是 OnP2CardActionTrigger 的核心：解析按钮 value，识别
// 操作人，调 ApproveWithLock，给 admin 卡片同步替换 + 给申请人发结果卡。
//
// 重要约定:
//   - 必须在 3 秒内返回；DB / contact API 都是内网/同区，主路径只有 1 次
//     contact API + 1 次 SQL update + 构卡。给申请人下发结果卡走 goroutine
//     避免阻塞响应。
//   - 重复点击 / 双 admin 抢按 / 已超时关闭：ApproveWithLock 返回 rows=0，
//     回 toast「工单已被处理」，原卡片不动。
func handleCardAction(ctx context.Context, event *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
	if event == nil || event.Event == nil || event.Event.Action == nil {
		return errToast("回调数据缺失"), nil
	}
	value := event.Event.Action.Value
	action, _ := value["action"].(string)
	approvalID, ok := readApprovalID(value["approval_id"])
	if action == "" || !ok {
		return errToast("按钮参数不合法"), nil
	}

	var newStatus int
	var resultKind string
	switch action {
	case "approve":
		newStatus = 4
		resultKind = "approved"
	case "reject":
		newStatus = 2
		resultKind = "rejected"
	default:
		return errToast(fmt.Sprintf("未知按钮 action: %v", action)), nil
	}

	openID := ""
	if event.Event.Operator != nil {
		openID = event.Event.Operator.OpenID
	}
	if openID == "" {
		return errToast("无法识别操作人（open_id 为空）"), nil
	}

	adminName, err := resolveAdminByOpenId(openID)
	if err != nil {
		log.Logger.Errorf("resolve admin by open_id(%v) err: %v", openID, err)
		return errToast("识别操作人失败，请联系运维"), nil
	}
	if adminName == "" {
		return errToast("您不是 admin，无权审批"), nil
	}

	rows, err := db.ApproveWithLock(approvalID, newStatus, adminName)
	if err != nil {
		log.Logger.Errorf("approve-with-lock id=%v status=%v admin=%v err: %v", approvalID, newStatus, adminName, err)
		return errToast("数据库写入失败，请联系运维"), nil
	}
	if rows == 0 {
		return &callback.CardActionTriggerResponse{
			Toast: &callback.Toast{
				Type:    "info",
				Content: "工单已被处理",
			},
		}, nil
	}

	approval, _ := db.GetTacacsApprovalByID(approvalID)
	if approval == nil {
		log.Logger.Errorf("approval id=%v vanished after ApproveWithLock", approvalID)
		return successToast("已记录，但工单详情读取失败"), nil
	}

	go func(a *db.TacacsApproval, kind, op string) {
		if err := feishu.SendCardToPersonByTacacsUserName(
			[]string{a.User},
			feishu.BuildApprovalResultCard(a, kind, op),
		); err != nil {
			log.Logger.Errorf("notify applicant(%v) result card err: %v", a.User, err)
		}
	}(approval, resultKind, adminName)

	return &callback.CardActionTriggerResponse{
		Toast: &callback.Toast{
			Type:    "success",
			Content: toastForKind(resultKind),
		},
		Card: &callback.Card{
			Type: "raw",
			Data: feishu.BuildApprovalResultCard(approval, resultKind, adminName),
		},
	}, nil
}

// readApprovalID 接受 float64（JSON number 默认）/int64/string 三种形态。
func readApprovalID(v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		return int64(x), true
	case int64:
		return x, true
	case int:
		return int64(x), true
	case string:
		var id int64
		_, err := fmt.Sscanf(x, "%d", &id)
		return id, err == nil
	}
	return 0, false
}

// resolveAdminByOpenId 把回调里的 OpenID 反向匹配到 tacacs_admin。
//
// 走「按 admin 反查 open_id」而不是「open_id → email/mobile → 匹配」的原因：
// /contact/v3/users/{id} 接口的 email/mobile 字段需要 contact:user.email:readonly +
// contact:user.phone:readonly 字段级权限，缺失时返回 200 但字段被静默剥成 ""。
// batch_get_id 接口在企业自建应用默认权限里就能用（发卡片本身就在用），
// 拿到的 open_id 是飞书侧主键、字符串等比即可，没有字段隐藏问题。
//
// 命中 0 个返回 ""（非 admin）；命中 ≥2 个 log.Errorf 后取第一个。
// 单 admin reverse-lookup 失败只 log.Warning，不阻断其他 admin 匹配——
// 比如 admin A 的飞书帐号被禁用、查不到 open_id，不应妨碍 admin B 通过审批。
func resolveAdminByOpenId(openID string) (string, error) {
	admins := db.GetTacacsAdminUser()
	if len(admins) == 0 {
		return "", nil
	}
	matched := make([]string, 0, 1)
	for _, name := range admins {
		info, err := db.GetTacacsUserInfoByUserName(name)
		if err != nil || info == nil {
			continue
		}
		adminOpenId, err := feishu.GetOpenIdByBasicInfo(info.Email, info.PhoneNumber)
		if err != nil {
			log.Logger.Warningf("resolveAdminByOpenId: get open_id for admin %v err: %v", name, err)
			continue
		}
		if adminOpenId == openID {
			matched = append(matched, name)
		}
	}
	if len(matched) == 0 {
		return "", nil
	}
	if len(matched) > 1 {
		log.Logger.Errorf("multiple admins matched feishu user(open_id:%v): %v, picking first", openID, matched)
	}
	return matched[0], nil
}

func errToast(msg string) *callback.CardActionTriggerResponse {
	return &callback.CardActionTriggerResponse{
		Toast: &callback.Toast{
			Type:    "error",
			Content: msg,
		},
	}
}

func successToast(msg string) *callback.CardActionTriggerResponse {
	return &callback.CardActionTriggerResponse{
		Toast: &callback.Toast{
			Type:    "success",
			Content: msg,
		},
	}
}

func toastForKind(kind string) string {
	switch kind {
	case "approved":
		return "已通过"
	case "rejected":
		return "已驳回"
	default:
		return "已处理"
	}
}
