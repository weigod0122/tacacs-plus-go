package http

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"tacacs/pkg/server/clickhouse"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestScopeLogFiltersAddsVerifiedUserForOrdinaryUser(t *testing.T) {
	filters := []clickhouse.Filter{{Field: "switchAddr", Operator: "eq", Value: "10.0.0.1"}}

	got, err := scopeLogFilters(filters, false, "alice")
	if err != nil {
		t.Fatalf("scopeLogFilters returned error: %v", err)
	}
	want := []clickhouse.Filter{
		{Field: "switchAddr", Operator: "eq", Value: "10.0.0.1"},
		{Field: "user", Operator: "eq", Value: "alice"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scoped filters = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(filters, []clickhouse.Filter{{Field: "switchAddr", Operator: "eq", Value: "10.0.0.1"}}) {
		t.Fatalf("scopeLogFilters mutated caller filters: %#v", filters)
	}
}

func TestScopeLogFiltersRejectsOrdinaryUserUsernameFilter(t *testing.T) {
	_, err := scopeLogFilters([]clickhouse.Filter{{Field: "USER", Operator: "eq", Value: "bob"}}, false, "alice")
	if err != errLogUserFilterDenied {
		t.Fatalf("error = %v, want %v", err, errLogUserFilterDenied)
	}
}

func TestScopeLogFiltersAdminCanChooseUsernameFilter(t *testing.T) {
	want := []clickhouse.Filter{{Field: "user", Operator: "eq", Value: "bob"}}
	got, err := scopeLogFilters(want, true, "admin")
	if err != nil {
		t.Fatalf("scopeLogFilters returned error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("admin filters = %#v, want %#v", got, want)
	}
	if &got[0] == &want[0] {
		t.Fatal("admin filter slice aliases caller slice")
	}
}

func TestLogRequestIdentityRequiresVerifiedSignatureContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/tacacs/log/meta", nil)
	c.Request.Header.Set("X-SwM-User", "alice")
	c.Request.Header.Set("X-SwM-Is-Admin", "0")

	if _, _, err := logRequestIdentity(c); err != errLogIdentityUnverified {
		t.Fatalf("unverified identity error = %v, want %v", err, errLogIdentityUnverified)
	}

	c.Set(swmAuthVerifiedContextKey, true)
	username, isAdmin, err := logRequestIdentity(c)
	if err != nil {
		t.Fatalf("verified identity returned error: %v", err)
	}
	if username != "alice" || isAdmin {
		t.Fatalf("identity = (%q, %v), want (alice, false)", username, isAdmin)
	}
}
