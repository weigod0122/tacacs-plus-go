package http

import (
	"net/http"
	"strings"
	"tacacs/pkg/public/db"

	"github.com/gin-gonic/gin"
)

// requestIdentity returns the identity injected by SwM after a successful
// signature check. Handlers use this helper for row-level authorization instead
// of trusting a client-supplied header on its own.
func requestIdentity(c *gin.Context) (username string, isAdmin bool, verified bool) {
	username = strings.TrimSpace(c.GetHeader("X-SwM-User"))
	isAdmin = c.GetHeader("X-SwM-Is-Admin") == "1"
	verifiedValue, _ := c.Get(swmAuthVerifiedContextKey)
	verified, _ = verifiedValue.(bool)
	return username, isAdmin, verified
}

func requireVerifiedIdentity(c *gin.Context) (string, bool, bool) {
	username, isAdmin, verified := requestIdentity(c)
	if !verified || username == "" {
		return "", false, false
	}
	return username, isAdmin, true
}

func requireAdminIdentity(c *gin.Context) bool {
	_, isAdmin, verified := requireVerifiedIdentity(c)
	if !verified {
		c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "missing verified identity"})
		return false
	}
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "message": "admin privileges required"})
		return false
	}
	return true
}

func canReadOwnedRecord(username, owner string, isAdmin bool) bool {
	return isAdmin || (username != "" && username == owner)
}

func filterApprovalRowsForUser(rows []*db.TacacsApproval, username string, isAdmin bool) []*db.TacacsApproval {
	if isAdmin {
		return rows
	}
	filtered := make([]*db.TacacsApproval, 0, len(rows))
	for _, row := range rows {
		if row != nil && canReadOwnedRecord(username, row.User, false) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func publicRoleRows(rows []db.TacacsRoleTemplate) []db.TacacsRoleTemplate {
	publicRows := make([]db.TacacsRoleTemplate, 0, len(rows))
	for _, row := range rows {
		publicRows = append(publicRows, db.TacacsRoleTemplate{Template: row.Template})
	}
	return publicRows
}
