package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"tacacs/pkg/public/cfg"

	"github.com/gin-gonic/gin"
)

// getUserStatus asks the Server for the current authoritative web-account
// state.  The username is part of the signed path (rather than a query
// parameter) because the shared HMAC canonical form intentionally signs only
// URL paths; this prevents a valid internal signature being retargeted by
// changing a query string in transit.
func getUserStatus(username string) (string, error) {
	if strings.TrimSpace(username) == "" {
		return "", fmt.Errorf("empty username")
	}
	conf := cfg.SwmConfig()
	if conf == nil {
		return "", fmt.Errorf("swm config is not initialized")
	}
	endpoint := fmt.Sprintf("%s/internal/user/status/%s",
		strings.TrimRight(conf.TacacsManagerUrl, "/"),
		url.PathEscape(username))
	body, err := signedInternalGet(endpoint)
	if err != nil {
		return "", err
	}
	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode user status: %w", err)
	}
	if result.Status == "" {
		return "", fmt.Errorf("empty user status")
	}
	return result.Status, nil
}

func passwordResetPathAllowed(c *gin.Context) bool {
	if c.Request.Method == http.MethodGet && c.Request.URL.Path == "/" {
		return true
	}
	return c.Request.Method == http.MethodPost && c.Request.URL.Path == "/tacacs/user/update/password"
}
