package http

import (
	"testing"
	"time"
)

func TestAllowApprovalCreateBoundsPerUser(t *testing.T) {
	approvalQuota.Lock()
	approvalQuota.entries = make(map[string][]time.Time)
	approvalQuota.Unlock()
	now := time.Now()
	for i := 0; i < approvalCreateMax; i++ {
		if !allowApprovalCreate("alice", now.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("request %d was unexpectedly rejected", i+1)
		}
	}
	if allowApprovalCreate("alice", now.Add(30*time.Second)) {
		t.Fatal("request beyond per-user quota was accepted")
	}
	if !allowApprovalCreate("bob", now.Add(30*time.Second)) {
		t.Fatal("quota for alice affected bob")
	}
	if !allowApprovalCreate("alice", now.Add(approvalCreateWindow+time.Second)) {
		t.Fatal("expired quota entry was not evicted")
	}
}
