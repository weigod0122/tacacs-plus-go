package http

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPathRequiresAdminForTemplateEndpoints(t *testing.T) {
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

func TestProxyReadAndRestoreBodyRejectsOversizedBody(t *testing.T) {
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
