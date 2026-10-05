package http

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"tacacs/pkg/public/db"
	"tacacs/pkg/public/logquery"
	"tacacs/pkg/public/waitGroup"
	"tacacs/pkg/server/clickhouse"
	"time"

	"github.com/gin-gonic/gin"
)

type logMetaField struct {
	Key       string             `json:"key"`
	Label     string             `json:"label"`
	FieldName string             `json:"fieldName"`
	Name      string             `json:"name,omitempty"`
	Kind      logquery.FieldKind `json:"kind"`
}

type logMetaType struct {
	Type            string         `json:"type"`
	Visible         bool           `json:"visible"`
	Available       bool           `json:"available"`
	Table           string         `json:"table,omitempty"`
	EventTimeColumn string         `json:"eventTimeColumn,omitempty"`
	Fields          []logMetaField `json:"fields"`
}

type logMetaView struct {
	Mode            string            `json:"mode"`
	External        logRedirectConfig `json:"external"`
	ClickHouseReady bool              `json:"clickhouseReady"`
	Types           []logMetaType     `json:"types"`
}

type logQueryRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type logQueryRequest struct {
	Type         string              `json:"type"`
	EventRange   logQueryRange       `json:"eventRange"`
	Filters      []clickhouse.Filter `json:"filters"`
	Columns      []string            `json:"columns"`
	Page         int                 `json:"page"`
	PageSize     int                 `json:"pageSize"`
	IncludeTotal bool                `json:"includeTotal"`
}

func httpApiLogMeta(c *gin.Context) {
	if _, _, err := logRequestIdentity(c); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": err.Error()})
		return
	}
	external, err := loadLogRedirectConfig()
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{"code": http.StatusFailedDependency, "message": fmt.Sprintf("load log metadata failed: %v", err)})
		return
	}
	mode, err := db.GetLogDisplayMode()
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{"code": http.StatusFailedDependency, "message": fmt.Sprintf("load log display mode failed: %v", err)})
		return
	}
	if mode == "external" {
		c.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "data": logMetaView{
			Mode:     mode,
			External: external,
			Types:    nil,
		}})
		return
	}

	ckCfg, err := db.GetClickHouseLogConfig()
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{"code": http.StatusFailedDependency, "message": fmt.Sprintf("load clickhouse log config failed: %v", err)})
		return
	}
	ready := ckCfg.Address != "" && ckCfg.Database != ""
	types := make([]logMetaType, 0, len(logquery.AllLogTypes()))
	for _, typ := range logquery.AllLogTypes() {
		// The per-type visible* switches belong to external-link mode. Direct
		// ClickHouse mode always exposes all three tabs; row ownership is
		// enforced separately by httpApiLogQuery.
		visible := true
		mapping := ckCfg.Mappings[typ]
		fields := make([]logMetaField, 0, len(logquery.Specs(typ)))
		for _, spec := range logquery.Specs(typ) {
			fields = append(fields, logMetaField{
				Key:       spec.Key,
				Label:     logquery.DisplayLabel(typ, spec.Key, mapping),
				FieldName: logquery.DisplayLabel(typ, spec.Key, logquery.Mapping{}),
				Name:      strings.TrimSpace(mapping.Names[spec.Key]),
				Kind:      spec.Kind,
			})
		}
		types = append(types, logMetaType{
			Type:            typ.String(),
			Visible:         visible,
			Available:       ready && logquery.MappingComplete(typ, mapping),
			Table:           mapping.Table,
			EventTimeColumn: mapping.EventTimeColumn,
			Fields:          fields,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "data": logMetaView{
		Mode:            mode,
		External:        external,
		ClickHouseReady: ready,
		Types:           types,
	}})
}

func httpApiLogQuery(c *gin.Context) {
	waitGroup.GlobalWg.Add(1)
	defer waitGroup.GlobalWg.Done()

	username, isAdmin, identityErr := logRequestIdentity(c)
	if identityErr != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": identityErr.Error()})
		return
	}

	var body logQueryRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": fmt.Sprintf("invalid body: %v", err)})
		return
	}
	typ, err := logquery.ParseLogType(body.Type)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": err.Error()})
		return
	}
	mode, err := db.GetLogDisplayMode()
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{"code": http.StatusFailedDependency, "message": fmt.Sprintf("load log display mode failed: %v", err)})
		return
	}
	if mode != "clickhouse" {
		c.JSON(http.StatusConflict, gin.H{"code": http.StatusConflict, "message": "log display mode is external"})
		return
	}
	filters, err := scopeLogFilters(body.Filters, isAdmin, username)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "message": err.Error()})
		return
	}

	from, err := parseLogBoundary(body.EventRange.From)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": "invalid eventRange.from: " + err.Error()})
		return
	}
	to, err := parseLogBoundary(body.EventRange.To)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": "invalid eventRange.to: " + err.Error()})
		return
	}
	result, err := clickhouse.DefaultManager.Query(c.Request.Context(), clickhouse.QueryRequest{
		Type:         typ,
		From:         from,
		To:           to,
		Filters:      filters,
		Columns:      body.Columns,
		Page:         body.Page,
		PageSize:     body.PageSize,
		IncludeTotal: body.IncludeTotal,
	})
	if err != nil {
		c.JSON(http.StatusFailedDependency, gin.H{"code": http.StatusFailedDependency, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "data": result})
}

var (
	errLogIdentityUnverified = errors.New("request identity is not verified")
	errLogIdentityMissing    = errors.New("missing user identity")
	errLogAdminHeaderInvalid = errors.New("invalid admin identity")
	errLogUserFilterDenied   = errors.New("ordinary users cannot filter by username")
)

// logRequestIdentity is the second authorization gate for log APIs. The
// reverse proxy signs X-SwM-User/X-SwM-Is-Admin and swmAuthMiddleware records
// whether that signature was valid. Checking both the context marker and the
// headers prevents a caller from reaching these handlers with forged identity
// headers when swm_auth.enforce=false.
func logRequestIdentity(c *gin.Context) (string, bool, error) {
	verified, ok := c.Get(swmAuthVerifiedContextKey)
	if !ok || verified != true {
		return "", false, errLogIdentityUnverified
	}
	username := strings.TrimSpace(c.GetHeader("X-SwM-User"))
	if username == "" {
		return "", false, errLogIdentityMissing
	}
	adminHeader := strings.TrimSpace(c.GetHeader("X-SwM-Is-Admin"))
	if adminHeader != "0" && adminHeader != "1" {
		return "", false, errLogAdminHeaderInvalid
	}
	return username, adminHeader == "1", nil
}

// scopeLogFilters applies the server-owned row scope for ordinary users.
// Username is never accepted from an ordinary user's request body: the only
// user predicate is the identity carried by the verified SwM request.
func scopeLogFilters(filters []clickhouse.Filter, isAdmin bool, username string) ([]clickhouse.Filter, error) {
	if isAdmin {
		return append([]clickhouse.Filter(nil), filters...), nil
	}
	if strings.TrimSpace(username) == "" {
		return nil, errLogIdentityMissing
	}

	scoped := make([]clickhouse.Filter, 0, len(filters)+1)
	for _, filter := range filters {
		if strings.EqualFold(strings.TrimSpace(filter.Field), "user") {
			return nil, errLogUserFilterDenied
		}
		scoped = append(scoped, filter)
	}
	scoped = append(scoped, clickhouse.Filter{Field: "user", Operator: "eq", Value: username})
	return scoped, nil
}

func isLogTypeVisible(cfg logRedirectConfig, typ logquery.LogType) bool {
	switch typ {
	case logquery.LogTypeAuthen:
		return cfg.VisibleAuthen
	case logquery.LogTypeAuthor:
		return cfg.VisibleAuthor
	case logquery.LogTypeAccount:
		return cfg.VisibleAccount
	default:
		return false
	}
}

func parseLogBoundary(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("value is required")
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, nil
	}
	return time.ParseInLocation("2006-01-02 15:04:05", raw, time.Local)
}
