package permissionSystem

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"tacacs/pkg/public/apolloConfig"
	"tacacs/pkg/public/db"
	"tacacs/pkg/public/env"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/notify/feishu"
	"tacacs/pkg/public/utils"
	"tacacs/pkg/server/approvalSystem"
	"time"

	"github.com/apolloconfig/agollo/v5/storage"
	"github.com/bytedance/sonic"
)

const (
	UserPermissionUpdateCycle      = 2    //用户权限更新周期，单位秒
	PasswordTimeoutCheckCycle      = 60   //用户密码超时检查周期，单位秒
	PasswordUnTimeoutCheckCycle    = 10   //密码已更改、恢复权限检查周期，单位秒
	RolePermissionExpiryCheckCycle = 1800 //角色权限即将到期检查周期，单位秒。30 分钟

	PasswordTimeout      = 7776000 //密码超时时间，单位秒。90 天
	PasswordTimeoutAlarm = 7171200 //密码超时提醒阈值，单位秒。距过期 < 7 天开始提醒

	// 单用户告警去重窗口：进入提醒区间后每 24h 最多发一次卡片
	passwordAlarmDedupWindow   = 24 * time.Hour
	roleExpiryAlarmDedupWindow = 24 * time.Hour

	// 飞书发卡片的并发上限，防止上游限流
	feishuSendConcurrency = 5

	// 角色权限即将到期提醒的三档阈值
	roleExpiryTier3Day   = 72 * time.Hour
	roleExpiryTier1Day   = 24 * time.Hour
	roleExpiryTier12Hour = 12 * time.Hour
)

const (
	userStatusNormal = "1"
	userStatusPause  = "2"
	userStatusDelete = "0"
)

type TacacsPermissions struct {
	StartTime   time.Time
	EndTime     time.Time
	Permissions string
}

// PermissionSystem 守护进程：周期性同步用户/角色，检查密码到期，刷新值班人员。
//
// 并发模型：
//   - userInfos / permissionsInfo 用 atomic.Pointer 发布，写者构造好新 map 后一次性 Store；
//     读者 Load 拿到的是某一时刻的不可变快照，全程无锁。
//   - run 用 atomic.Bool，避免锁。
//   - alarmHistory 用 TypedSyncMap 自管线程安全；只在 CheckPasswordTimeout 内读写。
//   - 不再有 globalMutex，4 个 daemon task 真正并行。
type PermissionSystem struct {
	userInfos              atomic.Pointer[map[string]*db.TacacsUser]
	permissionsInfo        atomic.Pointer[map[string]map[string][]TacacsPermissions]
	alarmHistory           *utils.TypedSyncMap[string, time.Time] // 密码到期告警去重: user -> last alarm sent
	roleExpiryAlarmHistory *utils.TypedSyncMap[string, time.Time] // 角色到期告警去重: dedupKey(user,level,endTime,tier) -> last alarm sent
	run                    atomic.Bool
}

// NewPermissionSystem 构造并启动后台守护进程。Start() 之后任务才真正生效。
func NewPermissionSystem() *PermissionSystem {
	ps := &PermissionSystem{
		alarmHistory:           utils.NewTypedSyncMap[string, time.Time](),
		roleExpiryAlarmHistory: utils.NewTypedSyncMap[string, time.Time](),
	}
	emptyUsers := make(map[string]*db.TacacsUser)
	emptyPerms := make(map[string]map[string][]TacacsPermissions)
	ps.userInfos.Store(&emptyUsers)
	ps.permissionsInfo.Store(&emptyPerms)
	go ps.running()
	return ps
}

func (ps *PermissionSystem) Start() { ps.run.Store(true) }
func (ps *PermissionSystem) Stop()  { ps.run.Store(false) }

// loadUsers 返回当前用户快照。永不为 nil（init 时已 Store 过空 map）。
func (ps *PermissionSystem) loadUsers() map[string]*db.TacacsUser {
	return *ps.userInfos.Load()
}

// loadPermissions 返回当前权限快照。
func (ps *PermissionSystem) loadPermissions() map[string]map[string][]TacacsPermissions {
	return *ps.permissionsInfo.Load()
}

// UpdateUserInfos 全量拉取 tacacs_user，无锁切换。
func (ps *PermissionSystem) UpdateUserInfos() {
	users, err := db.GetTacacsUserInfos()
	if err != nil {
		log.Logger.Errorf("UpdateUserInfos: GetTacacsUserInfos failed: %v", err)
		return
	}

	newMap := make(map[string]*db.TacacsUser, len(users))
	for _, u := range users {
		newMap[u.User] = u
	}
	ps.userInfos.Store(&newMap)
}

// UpdateRoleInfos 全量拉取已通过审批，按用户 + 权限分桶后合并时间区间。
func (ps *PermissionSystem) UpdateRoleInfos() {
	approvals, err := db.GetTacacsApproval(approvalSystem.ApprovalPass)
	if err != nil {
		log.Logger.Errorf("UpdateRoleInfos: GetTacacsApproval failed: %v", err)
		return
	}

	users := ps.loadUsers()
	temp := make(map[string]map[string][]TacacsPermissions)
	for _, a := range approvals {
		if _, exists := users[a.User]; !exists {
			continue
		}
		if temp[a.User] == nil {
			temp[a.User] = make(map[string][]TacacsPermissions)
		}
		temp[a.User][a.ApprovalPermissions] = append(
			temp[a.User][a.ApprovalPermissions],
			TacacsPermissions{
				StartTime:   a.StartTime,
				EndTime:     a.EndTime,
				Permissions: a.ApprovalPermissions,
			},
		)
	}

	for user := range temp {
		temp[user] = mergePermissions(temp[user])
	}

	ps.permissionsInfo.Store(&temp)
}

func mergePermissions(permissions map[string][]TacacsPermissions) map[string][]TacacsPermissions {
	result := make(map[string][]TacacsPermissions, len(permissions))
	for level, ranges := range permissions {
		result[level] = mergeRangeTimes(ranges)
	}
	return result
}

// mergeRangeTimes 合并按开始时间升序后的相邻/重叠时间段。
// 端点严格相等（无缝接续，例 [a,b] [b,c]）也合并成 [a,c]，否则
// CheckRolePermissionExpiry 会在 b 临近时假报"权限即将到期"。
func mergeRangeTimes(times []TacacsPermissions) []TacacsPermissions {
	if len(times) == 0 {
		return []TacacsPermissions{}
	}
	sort.Slice(times, func(i, j int) bool {
		return times[i].StartTime.Before(times[j].StartTime)
	})

	result := make([]TacacsPermissions, 0, len(times))
	current := times[0]
	for i := 1; i < len(times); i++ {
		if times[i].StartTime.After(current.EndTime) {
			result = append(result, current)
			current = times[i]
		} else if times[i].EndTime.After(current.EndTime) {
			current.EndTime = times[i].EndTime
		}
	}
	return append(result, current)
}

// GetCurrentUserRole 计算用户在「当前时间」生效的所有角色，逗号 join 返回。
// 没有角色返回 "null"（与历史行为兼容，调用方依赖此字面量做比较）。
func (ps *PermissionSystem) GetCurrentUserRole(user string) string {
	return getCurrentUserRoleAt(ps.loadPermissions(), user, time.Now())
}

// getCurrentUserRoleAt 把"当前时间"参数化，便于单元测试。
func getCurrentUserRoleAt(perms map[string]map[string][]TacacsPermissions, user string, now time.Time) string {
	userPerms, ok := perms[user]
	if !ok {
		return "null"
	}
	roles := make([]string, 0, len(userPerms))
	for level, ranges := range userPerms {
		for _, r := range ranges {
			if now.After(r.StartTime) && now.Before(r.EndTime) {
				roles = append(roles, level)
				break // 同一 level 只算一次
			}
		}
	}
	if len(roles) == 0 {
		return "null"
	}
	sort.Strings(roles)
	return strings.Join(roles, ",")
}

// UpdateDatabase 把"应有 role" 与 "DB 中 role" 不一致的用户 一次批量回写。
// 1000 个用户改角色：原来 1000 次 round-trip → 现在 1 次。
func (ps *PermissionSystem) UpdateDatabase() {
	users := ps.loadUsers()
	perms := ps.loadPermissions()
	now := time.Now()

	roleUpdates := make(map[string]string)
	for user, info := range users {
		expect := getCurrentUserRoleAt(perms, user, now)
		if info.Role != expect {
			roleUpdates[user] = expect
			log.Logger.Infof("%v`s tacacs role from %v to %v", user, info.Role, expect)
		}
	}

	if len(roleUpdates) == 0 {
		return
	}
	if err := db.BatchUpdateUserRole(roleUpdates); err != nil {
		log.Logger.Errorf("UpdateDatabase: BatchUpdateUserRole(%d users) failed: %v", len(roleUpdates), err)
	}
}

// CheckPasswordTimeout 检查密码距过期 < 7 天 / 已过期 的用户：
//   - 提醒区间：每 24h 最多发一次飞书卡片（通过 alarmHistory 去重）
//   - 已过期：批量改 status=Pause，并发限速发卡片
func (ps *PermissionSystem) CheckPasswordTimeout() {
	users := ps.loadUsers()
	now := time.Now()
	const maxDays = PasswordTimeout / (24 * 60 * 60)

	type alarmItem struct {
		user string
		days int
	}
	type expireItem struct {
		user string
	}
	var alarms []alarmItem
	var expires []expireItem

	for user, info := range users {
		if info.Status != userStatusNormal {
			continue
		}
		t, err := time.Parse("2006-01-02 15:04:05", info.PasswordUpdateTime)
		if err != nil {
			log.Logger.Warningf("CheckPasswordTimeout: parse PasswordUpdateTime for %v failed: %v (raw=%q)",
				user, err, info.PasswordUpdateTime)
			continue
		}
		duration := now.Sub(t)
		secs := duration.Seconds()

		if secs > PasswordTimeout {
			expires = append(expires, expireItem{user: user})
			continue
		}
		if secs > PasswordTimeoutAlarm {
			// 24h 去重
			if last, ok := ps.alarmHistory.Get(user); ok && now.Sub(last) < passwordAlarmDedupWindow {
				continue
			}
			alarms = append(alarms, alarmItem{user: user, days: int(duration.Hours() / 24)})
		}
	}

	// 1) 批量改状态
	if len(expires) > 0 {
		userList := make([]string, 0, len(expires))
		for _, e := range expires {
			userList = append(userList, e.user)
		}
		if err := db.BatchUpdateUserStatus(userList, userStatusPause); err != nil {
			log.Logger.Errorf("CheckPasswordTimeout: BatchUpdateUserStatus(Pause, %d users) failed: %v",
				len(userList), err)
		} else {
			for _, u := range userList {
				log.Logger.Infof("user(%v) password timeout, status -> Pause", u)
			}
		}
	}

	// 2) 受控并发发飞书卡片
	if len(alarms) > 0 {
		sem := make(chan struct{}, feishuSendConcurrency)
		var wg sync.WaitGroup
		for _, a := range alarms {
			wg.Add(1)
			go func(it alarmItem) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				if err := feishu.SendCardToPersonByTacacsUserName(
					[]string{it.user},
					feishu.BuildPasswordCard(it.user, "warning", it.days, maxDays),
				); err != nil {
					log.Logger.Errorf("push password warning card to %v err: %v", it.user, err)
					return
				}
				ps.alarmHistory.Set(it.user, time.Now())
				log.Logger.Infof("TACACS用户(%v)的密码已使用%v天，超过%v天后账号权限将回收",
					it.user, it.days, maxDays)
			}(a)
		}
		wg.Wait()
	}

	// 3) 已过期的同步发"expired"卡片（不去重，仅在状态从 Normal 转 Pause 那一刻触发）
	if len(expires) > 0 {
		sem := make(chan struct{}, feishuSendConcurrency)
		var wg sync.WaitGroup
		for _, e := range expires {
			wg.Add(1)
			go func(user string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				if err := feishu.SendCardToPersonByTacacsUserName(
					[]string{user},
					feishu.BuildPasswordCard(user, "expired", 0, maxDays),
				); err != nil {
					log.Logger.Errorf("push password expired card to %v err: %v", user, err)
				}
			}(e.user)
		}
		wg.Wait()
	}
}

// CheckPasswordUnTimeout 已被 Pause 但密码已更新（duration < PasswordTimeout）的用户恢复 Normal。
//
// TODO: 当前以"Status==Pause"作为"密码超时"判据，未区分管理员手动 Pause 等其他原因；
//
//	后续业务扩展需要时，给 tacacs_user 加一列 pause_reason 区分。
func (ps *PermissionSystem) CheckPasswordUnTimeout() {
	users := ps.loadUsers()
	now := time.Now()

	var restored []string
	for user, info := range users {
		if info.Status != userStatusPause {
			continue
		}
		t, err := time.Parse("2006-01-02 15:04:05", info.PasswordUpdateTime)
		if err != nil {
			log.Logger.Warningf("CheckPasswordUnTimeout: parse PasswordUpdateTime for %v failed: %v (raw=%q)",
				user, err, info.PasswordUpdateTime)
			continue
		}
		if now.Sub(t).Seconds() < PasswordTimeout {
			restored = append(restored, user)
		}
	}

	if len(restored) == 0 {
		return
	}

	if err := db.BatchUpdateUserStatus(restored, userStatusNormal); err != nil {
		log.Logger.Errorf("CheckPasswordUnTimeout: BatchUpdateUserStatus(Normal, %d users) failed: %v",
			len(restored), err)
		return
	}

	// 状态恢复后，把告警去重历史也清掉，让用户下次再接近过期时还能正常收到提醒
	for _, u := range restored {
		ps.alarmHistory.Delete(u)
	}

	// 受控并发发"恢复"卡片
	sem := make(chan struct{}, feishuSendConcurrency)
	var wg sync.WaitGroup
	for _, u := range restored {
		wg.Add(1)
		go func(user string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := feishu.SendCardToPersonByTacacsUserName(
				[]string{user},
				feishu.BuildPasswordCard(user, "restored", 0, 0),
			); err != nil {
				log.Logger.Errorf("push password restored card to %v err: %v", user, err)
			}
			log.Logger.Infof("user(%v) password restored, status -> Normal", user)
		}(u)
	}
	wg.Wait()
}

// UpdateOnDutyUsers 解析 on_duty JSON，取交集（外部值班 ∩ 白名单）覆写 tacacs_on_duty，并把变化推飞书。
func (ps *PermissionSystem) UpdateOnDutyUsers(raw string) {
	var result struct {
		OnDuty []string `json:"on_duty"`
	}
	if err := sonic.UnmarshalString(raw, &result); err != nil {
		log.Logger.Errorf("UpdateOnDutyUsers: parse on_duty config: %v", err)
		return
	}

	whitelist, err := db.GetTacacsOnDutyUserWhiteList()
	if err != nil {
		log.Logger.Errorf("UpdateOnDutyUsers: GetTacacsOnDutyUserWhiteList failed: %v", err)
		return
	}

	intersection := utils.GetIntersection(result.OnDuty, whitelist)
	onDutyUsers := make([]string, 0, len(intersection))
	for k := range intersection {
		onDutyUsers = append(onDutyUsers, k)
	}

	needAdd, needDelete, err := db.CoverTacacsOnDutyUser(onDutyUsers)
	if err != nil {
		log.Logger.Errorf("UpdateOnDutyUsers: CoverTacacsOnDutyUser failed: %v", err)
		return
	}

	if len(needAdd) == 0 && len(needDelete) == 0 {
		return
	}
	current := db.GetTacacsOnDutyUser()
	admins := db.GetTacacsAdminUser()
	card := feishu.BuildOnDutyChangeCard(needAdd, needDelete, current)
	if err := feishu.SendCardToPersonByTacacsUserName(admins, card); err != nil {
		log.Logger.Errorf("push on-duty change card to %v err: %v", strings.Join(admins, ","), err)
	}
}

type onDutyListener struct{ ps *PermissionSystem }

func (l *onDutyListener) OnChange(event *storage.ChangeEvent) {
	if v, ok := event.Changes["on_duty"]; ok {
		raw, ok := v.NewValue.(string)
		if !ok {
			log.Logger.Errorf("on_duty change value is not string")
			return
		}
		log.Logger.Infof("on_duty config changed, updating duty users")
		l.ps.UpdateOnDutyUsers(raw)
	}
}

func (l *onDutyListener) OnNewestChange(_ *storage.FullChangeEvent) {}

// roleExpiryAlert 表示一条"角色权限即将到期"告警。
type roleExpiryAlert struct {
	user     string
	level    string
	expireAt time.Time
	tier     string // "3d" / "1d" / "12h"
}

// dedupKey 返回这条告警的去重键。带上 expireAt.Unix()，
// 续期产生新 EndTime 时视为新告警目标，允许再发；带上 tier 让三档独立 dedup。
func (a roleExpiryAlert) dedupKey() string {
	return a.user + "|" + a.level + "|" + strconv.FormatInt(a.expireAt.Unix(), 10) + "|" + a.tier
}

// computeRoleExpiryAlerts 纯函数：扫所有 (user, level)，找当前覆盖 now 的合并段，
// 根据 EndTime 距 now 落入哪一档给出告警。
//   - tier "12h": 距过期 ≤ 12h
//   - tier "1d":  12h < 距过期 ≤ 24h
//   - tier "3d":  24h < 距过期 ≤ 72h
//   - 其余不报
//
// 输入要求：perms 已经过 mergeRangeTimes 处理（同 level 内区间不相交且按 StartTime 升序）。
func computeRoleExpiryAlerts(perms map[string]map[string][]TacacsPermissions, now time.Time) []roleExpiryAlert {
	var alerts []roleExpiryAlert
	for user, byLevel := range perms {
		for level, ranges := range byLevel {
			for _, r := range ranges {
				if !(now.After(r.StartTime) && now.Before(r.EndTime)) {
					continue
				}
				remain := r.EndTime.Sub(now)
				var tier string
				switch {
				case remain <= roleExpiryTier12Hour:
					tier = "12h"
				case remain <= roleExpiryTier1Day:
					tier = "1d"
				case remain <= roleExpiryTier3Day:
					tier = "3d"
				}
				if tier != "" {
					alerts = append(alerts, roleExpiryAlert{
						user:     user,
						level:    level,
						expireAt: r.EndTime,
						tier:     tier,
					})
				}
				break // 同 level 至多一个段 cover now
			}
		}
	}
	return alerts
}

// CheckRolePermissionExpiry 检查每个用户每个角色的合并区间，对 cover now 且 EndTime 临近的发提醒卡片。
// 三档（3d/1d/12h）各自独立 24h dedup；不动 DB。
func (ps *PermissionSystem) CheckRolePermissionExpiry() {
	perms := ps.loadPermissions()
	now := time.Now()

	alerts := computeRoleExpiryAlerts(perms, now)
	if len(alerts) == 0 {
		return
	}

	// 24h dedup
	pending := alerts[:0]
	for _, a := range alerts {
		key := a.dedupKey()
		if last, ok := ps.roleExpiryAlarmHistory.Get(key); ok && now.Sub(last) < roleExpiryAlarmDedupWindow {
			continue
		}
		pending = append(pending, a)
	}
	if len(pending) == 0 {
		return
	}

	sem := make(chan struct{}, feishuSendConcurrency)
	var wg sync.WaitGroup
	for _, a := range pending {
		wg.Add(1)
		go func(it roleExpiryAlert) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := feishu.SendCardToPersonByTacacsUserName(
				[]string{it.user},
				feishu.BuildRoleExpiryCard(it.user, it.level, it.tier, it.expireAt),
			); err != nil {
				log.Logger.Errorf("push role-expiry card to %v (level=%v, tier=%v) err: %v",
					it.user, it.level, it.tier, err)
				return
			}
			ps.roleExpiryAlarmHistory.Set(it.dedupKey(), time.Now())
			log.Logger.Infof("TACACS用户(%v)的角色(%v)将在 %v 到期 (tier=%v)",
				it.user, it.level, it.expireAt.Format("2006-01-02 15:04:05"), it.tier)
		}(a)
	}
	wg.Wait()
}

// runTask 每 cycle 跑一次 task，自身不持锁。
// task 内部如需共享状态请走 atomic.Pointer 快照。
func (ps *PermissionSystem) runTask(task func(), cycle time.Duration) {
	for {
		if ps.run.Load() {
			startTime := time.Now()
			task()
			if env.DEBUG {
				pkgName, fnName := utils.GetFunctionName(task)
				log.DebugLog("task(packageName:'%v', functionName:'%v') cost:%v", pkgName, fnName, time.Since(startTime))
			}
		}
		time.Sleep(cycle)
	}
}

// periodicUserUpdate 拉用户 -> 拉审批 -> 算 diff -> 批量回写 role
func (ps *PermissionSystem) periodicUserUpdate() {
	ps.UpdateUserInfos()
	ps.UpdateRoleInfos()
	ps.UpdateDatabase()
}

func (ps *PermissionSystem) running() {
	tasks := []struct {
		fn    func()
		cycle time.Duration
	}{
		{ps.periodicUserUpdate, time.Duration(UserPermissionUpdateCycle) * time.Second},
		{ps.CheckPasswordTimeout, time.Duration(PasswordTimeoutCheckCycle) * time.Second},
		{ps.CheckPasswordUnTimeout, time.Duration(PasswordUnTimeoutCheckCycle) * time.Second},
		{ps.CheckRolePermissionExpiry, time.Duration(RolePermissionExpiryCheckCycle) * time.Second},
	}
	for _, t := range tasks {
		go ps.runTask(t.fn, t.cycle)
	}

	if apolloConfig.IsBeSet() {
		if raw, err := apolloConfig.GetConfig("on_duty"); err == nil {
			ps.UpdateOnDutyUsers(raw)
		}
		apolloConfig.AddChangeListener(&onDutyListener{ps: ps})
	}
}
