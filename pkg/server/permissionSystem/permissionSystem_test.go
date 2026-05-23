package permissionSystem

import (
	"sort"
	"testing"
	"time"
)

// 一个简洁的固定基准时间，所有测试时间相对于它构造
var base = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func t(offsetMin int) time.Time {
	return base.Add(time.Duration(offsetMin) * time.Minute)
}

// makeRange 构造一个权限段
func makeRange(startMin, endMin int, perm string) TacacsPermissions {
	return TacacsPermissions{
		StartTime:   t(startMin),
		EndTime:     t(endMin),
		Permissions: perm,
	}
}

// rangesEqual 比较两组时间段是否一致（不要求顺序）
func rangesEqual(a, b []TacacsPermissions) bool {
	if len(a) != len(b) {
		return false
	}
	cp := func(in []TacacsPermissions) []TacacsPermissions {
		out := make([]TacacsPermissions, len(in))
		copy(out, in)
		sort.Slice(out, func(i, j int) bool {
			if !out[i].StartTime.Equal(out[j].StartTime) {
				return out[i].StartTime.Before(out[j].StartTime)
			}
			return out[i].EndTime.Before(out[j].EndTime)
		})
		return out
	}
	a = cp(a)
	b = cp(b)
	for i := range a {
		if !a[i].StartTime.Equal(b[i].StartTime) ||
			!a[i].EndTime.Equal(b[i].EndTime) ||
			a[i].Permissions != b[i].Permissions {
			return false
		}
	}
	return true
}

// =============== mergeRangeTimes ===============

func TestMergeRangeTimes_Empty(t1 *testing.T) {
	got := mergeRangeTimes(nil)
	if len(got) != 0 {
		t1.Fatalf("expected empty, got %v", got)
	}
	got = mergeRangeTimes([]TacacsPermissions{})
	if len(got) != 0 {
		t1.Fatalf("expected empty, got %v", got)
	}
}

func TestMergeRangeTimes_Single(t1 *testing.T) {
	in := []TacacsPermissions{makeRange(0, 60, "L1")}
	got := mergeRangeTimes(in)
	if !rangesEqual(got, in) {
		t1.Fatalf("expected %v, got %v", in, got)
	}
}

func TestMergeRangeTimes_NoOverlap(t1 *testing.T) {
	in := []TacacsPermissions{
		makeRange(0, 30, "L1"),
		makeRange(60, 90, "L1"),
	}
	got := mergeRangeTimes(in)
	if !rangesEqual(got, in) {
		t1.Fatalf("expected unchanged %v, got %v", in, got)
	}
}

func TestMergeRangeTimes_FullOverlap(t1 *testing.T) {
	// [0,60] 和 [10,30] 完全重叠 -> [0,60]
	in := []TacacsPermissions{
		makeRange(0, 60, "L1"),
		makeRange(10, 30, "L1"),
	}
	want := []TacacsPermissions{makeRange(0, 60, "L1")}
	got := mergeRangeTimes(in)
	if !rangesEqual(got, want) {
		t1.Fatalf("expected %v, got %v", want, got)
	}
}

func TestMergeRangeTimes_PartialOverlap(t1 *testing.T) {
	// [0,30] + [20,60] -> [0,60]
	in := []TacacsPermissions{
		makeRange(0, 30, "L1"),
		makeRange(20, 60, "L1"),
	}
	want := []TacacsPermissions{makeRange(0, 60, "L1")}
	got := mergeRangeTimes(in)
	if !rangesEqual(got, want) {
		t1.Fatalf("expected %v, got %v", want, got)
	}
}

func TestMergeRangeTimes_EndpointTouch(t1 *testing.T) {
	// 端点接触：[0,30] 和 [30,60] 视为无缝接续，应合并成 [0,60]。
	// （这一行为修复了误报"工单到期"假警报：在 30 时刻并未真正失效，60 才失效。）
	in := []TacacsPermissions{
		makeRange(0, 30, "L1"),
		makeRange(30, 60, "L1"),
	}
	want := []TacacsPermissions{makeRange(0, 60, "L1")}
	got := mergeRangeTimes(in)
	if !rangesEqual(got, want) {
		t1.Fatalf("expected merged %v, got %v", want, got)
	}
}

func TestMergeRangeTimes_UnsortedInput(t1 *testing.T) {
	// 输入乱序：[60,90] [0,30] [20,40] -> 先排序再合并: [0,30]+[20,40]=[0,40], [60,90]
	in := []TacacsPermissions{
		makeRange(60, 90, "L1"),
		makeRange(0, 30, "L1"),
		makeRange(20, 40, "L1"),
	}
	want := []TacacsPermissions{
		makeRange(0, 40, "L1"),
		makeRange(60, 90, "L1"),
	}
	got := mergeRangeTimes(in)
	if !rangesEqual(got, want) {
		t1.Fatalf("expected %v, got %v", want, got)
	}
}

func TestMergeRangeTimes_NestedShorter(t1 *testing.T) {
	// 短段被长段完全覆盖：[0,100] 包含 [10,20] -> 留 [0,100]
	in := []TacacsPermissions{
		makeRange(0, 100, "L1"),
		makeRange(10, 20, "L1"),
	}
	want := []TacacsPermissions{makeRange(0, 100, "L1")}
	got := mergeRangeTimes(in)
	if !rangesEqual(got, want) {
		t1.Fatalf("expected %v, got %v", want, got)
	}
}

// =============== getCurrentUserRoleAt ===============

// helper：构造单用户的 permissions map
func makePerms(user string, byLevel map[string][]TacacsPermissions) map[string]map[string][]TacacsPermissions {
	return map[string]map[string][]TacacsPermissions{user: byLevel}
}

func TestGetCurrentUserRoleAt_UserNotFound(t1 *testing.T) {
	perms := makePerms("alice", map[string][]TacacsPermissions{
		"L1": {makeRange(0, 60, "L1")},
	})
	got := getCurrentUserRoleAt(perms, "bob", t(30))
	if got != "null" {
		t1.Fatalf("expected null for missing user, got %q", got)
	}
}

func TestGetCurrentUserRoleAt_NowOutsideAllRanges(t1 *testing.T) {
	perms := makePerms("alice", map[string][]TacacsPermissions{
		"L1": {makeRange(0, 60, "L1")},
	})
	// now = t(120) > end
	got := getCurrentUserRoleAt(perms, "alice", t(120))
	if got != "null" {
		t1.Fatalf("expected null when now after all ranges, got %q", got)
	}
	// now = t(-30) < start
	got = getCurrentUserRoleAt(perms, "alice", t(-30))
	if got != "null" {
		t1.Fatalf("expected null when now before all ranges, got %q", got)
	}
}

func TestGetCurrentUserRoleAt_SingleRoleActive(t1 *testing.T) {
	perms := makePerms("alice", map[string][]TacacsPermissions{
		"L1": {makeRange(0, 60, "L1")},
	})
	got := getCurrentUserRoleAt(perms, "alice", t(30))
	if got != "L1" {
		t1.Fatalf("expected L1, got %q", got)
	}
}

func TestGetCurrentUserRoleAt_MultipleRolesSorted(t1 *testing.T) {
	// L2 和 L1 都在 t(30) 生效，期望按字母序：L1,L2
	perms := makePerms("alice", map[string][]TacacsPermissions{
		"L2": {makeRange(0, 60, "L2")},
		"L1": {makeRange(0, 60, "L1")},
	})
	got := getCurrentUserRoleAt(perms, "alice", t(30))
	if got != "L1,L2" {
		t1.Fatalf("expected sorted L1,L2 got %q", got)
	}
}

func TestGetCurrentUserRoleAt_OnlySomeLevelsActive(t1 *testing.T) {
	// L1 已过期，L2 在 t(50) 生效
	perms := makePerms("alice", map[string][]TacacsPermissions{
		"L1": {makeRange(0, 30, "L1")},
		"L2": {makeRange(40, 80, "L2")},
	})
	got := getCurrentUserRoleAt(perms, "alice", t(50))
	if got != "L2" {
		t1.Fatalf("expected L2, got %q", got)
	}
}

func TestGetCurrentUserRoleAt_SameLevelMultipleRangesNotDuplicated(t1 *testing.T) {
	// 同一 level 有两段都覆盖 now，期望只算一次
	perms := makePerms("alice", map[string][]TacacsPermissions{
		"L1": {
			makeRange(0, 60, "L1"),
			makeRange(20, 40, "L1"),
		},
	})
	got := getCurrentUserRoleAt(perms, "alice", t(30))
	if got != "L1" {
		t1.Fatalf("expected L1 (no duplicate), got %q", got)
	}
}

func TestGetCurrentUserRoleAt_BoundaryExclusive(t1 *testing.T) {
	// 边界严格 exclusive：now == StartTime 不算生效，now == EndTime 也不算
	perms := makePerms("alice", map[string][]TacacsPermissions{
		"L1": {makeRange(0, 60, "L1")},
	})
	if got := getCurrentUserRoleAt(perms, "alice", t(0)); got != "null" {
		t1.Fatalf("expected null at start boundary, got %q", got)
	}
	if got := getCurrentUserRoleAt(perms, "alice", t(60)); got != "null" {
		t1.Fatalf("expected null at end boundary, got %q", got)
	}
}

// =============== mergePermissions ===============

func TestMergePermissions_PerLevelMerged(t1 *testing.T) {
	in := map[string][]TacacsPermissions{
		"L1": {
			makeRange(0, 30, "L1"),
			makeRange(20, 60, "L1"), // 与上面重叠
		},
		"L2": {
			makeRange(0, 10, "L2"),
			makeRange(50, 90, "L2"), // 不重叠
		},
	}
	got := mergePermissions(in)
	if !rangesEqual(got["L1"], []TacacsPermissions{makeRange(0, 60, "L1")}) {
		t1.Fatalf("L1 not merged correctly, got %v", got["L1"])
	}
	if !rangesEqual(got["L2"], []TacacsPermissions{
		makeRange(0, 10, "L2"),
		makeRange(50, 90, "L2"),
	}) {
		t1.Fatalf("L2 not preserved correctly, got %v", got["L2"])
	}
}

// =============== computeRoleExpiryAlerts ===============

// 测试用：构造距 now 还有 dur 时间结束的合并段（StartTime 任意远，足够 cover now）
func makeRangeEndingIn(dur time.Duration, perm string, now time.Time) TacacsPermissions {
	return TacacsPermissions{
		StartTime:   now.Add(-30 * 24 * time.Hour), // 远超过 now，确保 now 在 cover 区间内
		EndTime:     now.Add(dur),
		Permissions: perm,
	}
}

// alertsContain 返回 alerts 中是否存在 (user, level, tier) 这条
func alertsContain(alerts []roleExpiryAlert, user, level, tier string) bool {
	for _, a := range alerts {
		if a.user == user && a.level == level && a.tier == tier {
			return true
		}
	}
	return false
}

func TestComputeRoleExpiryAlerts_NotInAnyRange(t1 *testing.T) {
	now := time.Now()
	// 用户的合并段已经过期（EndTime 在 now 之前）
	perms := makePerms("alice", map[string][]TacacsPermissions{
		"L1": {{
			StartTime: now.Add(-48 * time.Hour),
			EndTime:   now.Add(-1 * time.Hour),
		}},
	})
	got := computeRoleExpiryAlerts(perms, now)
	if len(got) != 0 {
		t1.Fatalf("expected 0 alerts (already expired), got %d: %+v", len(got), got)
	}
}

func TestComputeRoleExpiryAlerts_FarFromExpiry(t1 *testing.T) {
	now := time.Now()
	// 距过期还有 7 天，远超 3 天阈值
	perms := makePerms("alice", map[string][]TacacsPermissions{
		"L1": {makeRangeEndingIn(7*24*time.Hour, "L1", now)},
	})
	got := computeRoleExpiryAlerts(perms, now)
	if len(got) != 0 {
		t1.Fatalf("expected 0 alerts (far from expiry), got %d: %+v", len(got), got)
	}
}

func TestComputeRoleExpiryAlerts_3DayTier(t1 *testing.T) {
	now := time.Now()
	// 距过期 48h，落入 (24h, 72h]
	perms := makePerms("alice", map[string][]TacacsPermissions{
		"L1": {makeRangeEndingIn(48*time.Hour, "L1", now)},
	})
	got := computeRoleExpiryAlerts(perms, now)
	if len(got) != 1 {
		t1.Fatalf("expected 1 alert, got %d: %+v", len(got), got)
	}
	if !alertsContain(got, "alice", "L1", "3d") {
		t1.Fatalf("expected (alice, L1, 3d), got %+v", got)
	}
}

func TestComputeRoleExpiryAlerts_1DayTier(t1 *testing.T) {
	now := time.Now()
	// 距过期 18h，落入 (12h, 24h]
	perms := makePerms("alice", map[string][]TacacsPermissions{
		"L1": {makeRangeEndingIn(18*time.Hour, "L1", now)},
	})
	got := computeRoleExpiryAlerts(perms, now)
	if len(got) != 1 || !alertsContain(got, "alice", "L1", "1d") {
		t1.Fatalf("expected 1 alert (alice, L1, 1d), got %+v", got)
	}
}

func TestComputeRoleExpiryAlerts_12HourTier(t1 *testing.T) {
	now := time.Now()
	// 距过期 6h，落入 (0, 12h]
	perms := makePerms("alice", map[string][]TacacsPermissions{
		"L1": {makeRangeEndingIn(6*time.Hour, "L1", now)},
	})
	got := computeRoleExpiryAlerts(perms, now)
	if len(got) != 1 || !alertsContain(got, "alice", "L1", "12h") {
		t1.Fatalf("expected 1 alert (alice, L1, 12h), got %+v", got)
	}
}

func TestComputeRoleExpiryAlerts_BoundaryExact(t1 *testing.T) {
	now := time.Now()
	// 边界点：=12h 应归 12h；=24h 应归 1d；=72h 应归 3d
	cases := []struct {
		dur  time.Duration
		want string
	}{
		{12 * time.Hour, "12h"},
		{24 * time.Hour, "1d"},
		{72 * time.Hour, "3d"},
	}
	for _, c := range cases {
		perms := makePerms("alice", map[string][]TacacsPermissions{
			"L1": {makeRangeEndingIn(c.dur, "L1", now)},
		})
		got := computeRoleExpiryAlerts(perms, now)
		if len(got) != 1 || got[0].tier != c.want {
			t1.Fatalf("dur=%v expected tier=%q, got %+v", c.dur, c.want, got)
		}
	}
}

func TestComputeRoleExpiryAlerts_MultiLevelMultiTier(t1 *testing.T) {
	now := time.Now()
	// alice 同时持有两个 level，分别落入不同档
	perms := makePerms("alice", map[string][]TacacsPermissions{
		"core-rw": {makeRangeEndingIn(2*time.Hour, "core-rw", now)},  // 12h tier
		"core-ro": {makeRangeEndingIn(36*time.Hour, "core-ro", now)}, // 3d tier (24h<36h<=72h)
		"safe":    {makeRangeEndingIn(7*24*time.Hour, "safe", now)},  // 不告警
	})
	got := computeRoleExpiryAlerts(perms, now)
	if len(got) != 2 {
		t1.Fatalf("expected 2 alerts, got %d: %+v", len(got), got)
	}
	if !alertsContain(got, "alice", "core-rw", "12h") {
		t1.Fatalf("expected (alice, core-rw, 12h), got %+v", got)
	}
	if !alertsContain(got, "alice", "core-ro", "3d") {
		t1.Fatalf("expected (alice, core-ro, 3d), got %+v", got)
	}
}

func TestComputeRoleExpiryAlerts_MultiUser(t1 *testing.T) {
	now := time.Now()
	perms := map[string]map[string][]TacacsPermissions{
		"alice": {"L1": {makeRangeEndingIn(6*time.Hour, "L1", now)}},    // 12h
		"bob":   {"L1": {makeRangeEndingIn(48*time.Hour, "L1", now)}},   // 3d
		"carol": {"L1": {makeRangeEndingIn(7*24*time.Hour, "L1", now)}}, // 不告警
	}
	got := computeRoleExpiryAlerts(perms, now)
	if len(got) != 2 {
		t1.Fatalf("expected 2 alerts, got %d: %+v", len(got), got)
	}
	if !alertsContain(got, "alice", "L1", "12h") {
		t1.Fatalf("expected alice 12h, got %+v", got)
	}
	if !alertsContain(got, "bob", "L1", "3d") {
		t1.Fatalf("expected bob 3d, got %+v", got)
	}
}

func TestComputeRoleExpiryAlerts_AfterMerge(t1 *testing.T) {
	// 回归测试：两段端点接触的工单合并后，告警按合并段的 EndTime 算（远在 3 天阈值之外）。
	// 如果 merge bug 没修，会按第一段 EndTime（即"现在"）算，假报为已过期不告警 —— 仍 0 条但理由错。
	// 严格回归：用合并后 EndTime 落入 3d 档来构造测试，确保 merge 真实生效。
	now := time.Now()
	in := []TacacsPermissions{
		// 第一段：刚过期 1h（now 不在内）
		{StartTime: now.Add(-2 * time.Hour), EndTime: now.Add(-1 * time.Hour), Permissions: "L1"},
		// 第二段：从 -1h 到 now+48h，端点跟第一段相接
		{StartTime: now.Add(-1 * time.Hour), EndTime: now.Add(48 * time.Hour), Permissions: "L1"},
	}
	merged := mergeRangeTimes(in)
	if len(merged) != 1 {
		t1.Fatalf("expected merged into 1 segment, got %d: %+v", len(merged), merged)
	}
	perms := map[string]map[string][]TacacsPermissions{
		"alice": {"L1": merged},
	}
	got := computeRoleExpiryAlerts(perms, now)
	if len(got) != 1 || !alertsContain(got, "alice", "L1", "3d") {
		t1.Fatalf("expected alice L1 3d (post-merge), got %+v", got)
	}
}

func TestRoleExpiryAlertDedupKey_DifferentTiersDifferentKeys(t1 *testing.T) {
	// 同 user/level/endTime 的不同档应各自独立 dedup
	end := time.Now().Add(24 * time.Hour)
	a := roleExpiryAlert{user: "alice", level: "L1", expireAt: end, tier: "1d"}
	b := roleExpiryAlert{user: "alice", level: "L1", expireAt: end, tier: "3d"}
	if a.dedupKey() == b.dedupKey() {
		t1.Fatalf("different tiers must yield different dedup keys")
	}
}

func TestRoleExpiryAlertDedupKey_DifferentEndTimes(t1 *testing.T) {
	// 用户续期后 endTime 变化，新 endTime 视为新告警目标
	a := roleExpiryAlert{user: "alice", level: "L1", expireAt: time.Now().Add(24 * time.Hour), tier: "1d"}
	b := roleExpiryAlert{user: "alice", level: "L1", expireAt: time.Now().Add(48 * time.Hour), tier: "1d"}
	if a.dedupKey() == b.dedupKey() {
		t1.Fatalf("different endTimes must yield different dedup keys (allow re-alert after renewal)")
	}
}
