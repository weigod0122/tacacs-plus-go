package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPasswordResetPathAllowed(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		{name: "render reset page", method: http.MethodGet, path: "/", want: true},
		{name: "save password", method: http.MethodPost, path: "/tacacs/user/update/password", want: true},
		{name: "read approvals", method: http.MethodGet, path: "/tacacs/approval/get", want: false},
		{name: "post another endpoint", method: http.MethodPost, path: "/tacacs/user/check", want: false},
		{name: "post root", method: http.MethodPost, path: "/", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(tt.method, tt.path, nil)
			if got := passwordResetPathAllowed(c); got != tt.want {
				t.Fatalf("passwordResetPathAllowed(%s %s) = %v, want %v", tt.method, tt.path, got, tt.want)
			}
		})
	}
}
