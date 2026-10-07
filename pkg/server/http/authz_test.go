package http

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"reflect"
	"tacacs/pkg/public/db"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPathRequiresAdminForTemplateWritesAndContents(t *testing.T) {
	tests := []struct {
		path   string
		method string
		want   bool
	}{
		{path: "/tacacs/template/command/get", method: http.MethodGet, want: true},
		{path: "/tacacs/template/server/get", method: http.MethodGet, want: true},
		{path: "/tacacs/template/role/get", method: http.MethodGet, want: false},
		{path: "/tacacs/template/role/create", method: http.MethodPost, want: true},
		{path: "/tacacs/template/role/delete", method: http.MethodDelete, want: true},
	}
	for _, tt := range tests {
		if got := pathRequiresAdmin(tt.path, tt.method); got != tt.want {
			t.Errorf("pathRequiresAdmin(%q, %q) = %v, want %v", tt.path, tt.method, got, tt.want)
		}
	}
}

func TestReadAndRestoreBodyRejectsOversizedBody(t *testing.T) {
	body := bytes.Repeat([]byte("x"), maxBodyPeek+1)
	r := httptest.NewRequest(http.MethodPost, "/tacacs/approval/update", bytes.NewReader(body))
	got, err := readAndRestoreBody(r)
	if err == nil {
		t.Fatal("readAndRestoreBody returned nil error for oversized body")
	}
	if got != nil {
		t.Fatalf("readAndRestoreBody returned body for oversized body: %d bytes", len(got))
	}
	if r.Body == nil {
		t.Fatal("readAndRestoreBody did not restore request body")
	}
}

func TestFilterApprovalRowsForUser(t *testing.T) {
	rows := []*db.TacacsApproval{
		{ID: 1, User: "alice"},
		{ID: 2, User: "bob"},
		nil,
	}
	got := filterApprovalRowsForUser(rows, "alice", false)
	want := []*db.TacacsApproval{rows[0]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ordinary rows = %#v, want %#v", got, want)
	}
	if got := filterApprovalRowsForUser(rows, "alice", true); !reflect.DeepEqual(got, rows) {
		t.Fatalf("admin rows = %#v, want all rows %#v", got, rows)
	}
}

func TestPublicRoleRowsHideInternalMappings(t *testing.T) {
	rows := []db.TacacsRoleTemplate{{
		ID:                  7,
		Template:            "read-only",
		ServerTemplateList:  "prod-switches",
		CommandTemplateList: "show-commands",
	}}
	got := publicRoleRows(rows)
	want := []db.TacacsRoleTemplate{{Template: "read-only"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("public role rows = %#v, want %#v", got, want)
	}
	if rows[0].ServerTemplateList != "prod-switches" || rows[0].CommandTemplateList != "show-commands" {
		t.Fatal("publicRoleRows mutated source rows")
	}
}

func TestRequireAdminIdentityNeedsVerifiedContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name       string
		verified   bool
		admin      string
		wantStatus int
	}{
		{name: "missing signature", verified: false, admin: "1", wantStatus: http.StatusUnauthorized},
		{name: "ordinary user", verified: true, admin: "0", wantStatus: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/tacacs/template/server/get", nil)
			c.Request.Header.Set("X-SwM-User", "alice")
			c.Request.Header.Set("X-SwM-Is-Admin", tc.admin)
			if tc.verified {
				c.Set(swmAuthVerifiedContextKey, true)
			}
			if requireAdminIdentity(c) {
				t.Fatal("requireAdminIdentity unexpectedly allowed request")
			}
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantStatus)
			}
		})
	}
}
