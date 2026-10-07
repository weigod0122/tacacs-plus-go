package http

import (
	"strings"
	"sync"
	"time"
)

// Approval requests trigger a database write and an asynchronous Feishu
// notification. Keep a small process-local quota as a last line of defence;
// an edge/shared limiter can provide a stronger cross-replica policy later.
const (
	approvalCreateWindow  = 10 * time.Minute
	approvalCreateMax     = 20
	maxApprovalQuotaUsers = 100000
)

var approvalQuota = struct {
	sync.Mutex
	entries map[string][]time.Time
}{entries: make(map[string][]time.Time)}

func allowApprovalCreate(user string, now time.Time) bool {
	user = strings.TrimSpace(user)
	if user == "" {
		return false
	}
	approvalQuota.Lock()
	defer approvalQuota.Unlock()
	cutoff := now.Add(-approvalCreateWindow)
	entries := approvalQuota.entries[user]
	kept := entries[:0]
	for _, created := range entries {
		if created.After(cutoff) {
			kept = append(kept, created)
		}
	}
	if len(kept) >= approvalCreateMax {
		approvalQuota.entries[user] = kept
		return false
	}
	if _, exists := approvalQuota.entries[user]; !exists && len(approvalQuota.entries) >= maxApprovalQuotaUsers {
		for key, old := range approvalQuota.entries {
			fresh := old[:0]
			for _, created := range old {
				if created.After(cutoff) {
					fresh = append(fresh, created)
				}
			}
			if len(fresh) == 0 {
				delete(approvalQuota.entries, key)
			} else {
				approvalQuota.entries[key] = fresh
			}
		}
		if len(approvalQuota.entries) >= maxApprovalQuotaUsers {
			return false
		}
	}
	approvalQuota.entries[user] = append(kept, now)
	return true
}
