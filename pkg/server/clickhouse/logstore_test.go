package clickhouse

import (
	"strings"
	"tacacs/pkg/public/db"
	"tacacs/pkg/public/logquery"
	"testing"
	"time"
)

func TestBuildDSNQuotesCredentialsAndDatabase(t *testing.T) {
	dsn, err := buildDSN(structConfig("https://ck.example:8443", "ops user", "p@ss", "audit"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"@ck.example:8443/audit", "dial_timeout=5s", "read_timeout=30s"} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("dsn %q does not contain %q", dsn, want)
		}
	}
	if strings.Contains(dsn, "write_timeout") {
		t.Fatalf("dsn %q contains unsupported write_timeout setting", dsn)
	}
	if !strings.Contains(dsn, "ops%20user") || !strings.Contains(dsn, "p%40ss") {
		t.Fatalf("dsn %q did not URL-escape credentials", dsn)
	}
}

func TestFilterSQLUsesParameterizedValues(t *testing.T) {
	spec, ok := logquery.FieldSpecFor(logquery.LogTypeAuthen, "user")
	if !ok {
		t.Fatal("user spec missing")
	}
	query, arg, err := filterSQL(spec, Column{Name: "user_name", Type: "String"}, "contains", "' OR 1=1 --")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "?") {
		t.Fatalf("query %q does not use a placeholder", query)
	}
	if arg != "' OR 1=1 --" {
		t.Fatalf("arg = %#v", arg)
	}
}

func TestBuildWherePartsKeepsEventRangeInSeparateClause(t *testing.T) {
	typ := logquery.LogTypeAuthen
	mapping := logquery.Mapping{Fields: map[string]string{"user": "user_name"}}
	columns := map[string]Column{
		"event_time": {Name: "event_time", Type: "DateTime64(9, 'UTC')"},
		"user_name":  {Name: "user_name", Type: "String"},
	}
	req := QueryRequest{
		Type:    typ,
		From:    time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		To:      time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
		Filters: []Filter{{Field: "user", Operator: "eq", Value: "alice"}},
	}
	eventWhere, eventArgs, filterWhere, filterArgs, err := buildWhereParts(req, mapping, columns, columns["event_time"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(eventWhere, "`event_time` >= ?") || !strings.Contains(eventWhere, "`event_time` < ?") {
		t.Fatalf("eventWhere = %q", eventWhere)
	}
	if !strings.Contains(filterWhere, "`user_name` = ?") {
		t.Fatalf("filterWhere = %q", filterWhere)
	}
	if len(eventArgs) != 2 || len(filterArgs) != 1 || filterArgs[0] != "alice" {
		t.Fatalf("args = %#v %#v", eventArgs, filterArgs)
	}
}

func TestBuildQueryPartsPromotesProjectionKeyEquality(t *testing.T) {
	typ := logquery.LogTypeAuthen
	mapping := logquery.Mapping{Fields: map[string]string{
		"user": "user", "switchAddr": "switchAddr", "details": "details",
	}}
	columns := map[string]Column{
		"event_time": {Name: "event_time", Type: "DateTime64(9, 'UTC')"},
		"user":       {Name: "user", Type: "LowCardinality(String)"},
		"switchAddr": {Name: "switchAddr", Type: "LowCardinality(String)"},
		"details":    {Name: "details", Type: "String"},
	}
	req := QueryRequest{
		Type: typ,
		From: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
		Filters: []Filter{
			{Field: "switchAddr", Operator: "eq", Value: "switch-1"},
			{Field: "user", Operator: "eq", Value: "alice"},
			{Field: "details", Operator: "contains", Value: "timeout"},
		},
	}
	prewhere, preArgs, filterWhere, filterArgs, projection, err := buildQueryParts(req, mapping, columns, columns["event_time"])
	if err != nil {
		t.Fatal(err)
	}
	if projection != "p_by_switch_user_time" {
		t.Fatalf("projection = %q", projection)
	}
	if !strings.Contains(prewhere, "`switchAddr` = ?") || !strings.Contains(prewhere, "`user` = ?") {
		t.Fatalf("prewhere = %q", prewhere)
	}
	if strings.Contains(filterWhere, "`switchAddr`") || strings.Contains(filterWhere, "`user`") {
		t.Fatalf("projection key filter remained in WHERE: %q", filterWhere)
	}
	if !strings.Contains(filterWhere, "positionCaseInsensitive(toString(`details`), ?)") {
		t.Fatalf("filterWhere = %q", filterWhere)
	}
	if len(preArgs) != 4 || preArgs[2] != "switch-1" || preArgs[3] != "alice" {
		t.Fatalf("preArgs = %#v", preArgs)
	}
	if len(filterArgs) != 1 || filterArgs[0] != "timeout" {
		t.Fatalf("filterArgs = %#v", filterArgs)
	}
}

func TestBuildQueryPartsSkipsProjectionForNonEqualityOrRemappedKeys(t *testing.T) {
	typ := logquery.LogTypeAuthen
	columns := map[string]Column{
		"event_time": {Name: "event_time", Type: "DateTime64(9, 'UTC')"},
		"user_name":  {Name: "user_name", Type: "String"},
		"switchAddr": {Name: "switchAddr", Type: "String"},
	}
	req := QueryRequest{
		Type: typ,
		From: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
		Filters: []Filter{
			{Field: "user", Operator: "contains", Value: "ali"},
			{Field: "switchAddr", Operator: "eq", Value: "switch-1"},
		},
	}
	mapping := logquery.Mapping{Fields: map[string]string{"user": "user_name", "switchAddr": "switchAddr"}}
	prewhere, _, filterWhere, _, projection, err := buildQueryParts(req, mapping, columns, columns["event_time"])
	if err != nil {
		t.Fatal(err)
	}
	if projection != "p_by_switch_time" {
		t.Fatalf("projection = %q", projection)
	}
	if strings.Contains(prewhere, "`user_name`") {
		t.Fatalf("remapped user filter was promoted: %q", prewhere)
	}
	if !strings.Contains(filterWhere, "positionCaseInsensitive(toString(`user_name`), ?)") {
		t.Fatalf("filterWhere = %q", filterWhere)
	}
}

func TestSelectedColumnsPreserveRequestedOrder(t *testing.T) {
	typ := logquery.LogTypeAuthen
	mapping := logquery.Mapping{Fields: map[string]string{
		"time": "event_time", "timeStamp": "ts", "timeRange": "range", "user": "user_name",
		"switchAddr": "switch_addr", "serverAddr": "server_addr", "authenStatus": "status",
		"details": "details", "isSingleConnect": "single", "tacacsClient": "client",
	}}
	physical := map[string]Column{
		"event_time": {Name: "event_time", Type: "String"}, "ts": {Name: "ts", Type: "Int64"},
		"range": {Name: "range", Type: "Int64"}, "user_name": {Name: "user_name", Type: "String"},
		"switch_addr": {Name: "switch_addr", Type: "String"}, "server_addr": {Name: "server_addr", Type: "String"},
		"status": {Name: "status", Type: "String"}, "details": {Name: "details", Type: "String"},
		"single": {Name: "single", Type: "Bool"}, "client": {Name: "client", Type: "String"},
	}
	selected, err := selectedColumns(typ, mapping, physical, []string{"user", "time"})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 || selected[0].Key != "user" || selected[1].Key != "time" {
		t.Fatalf("selected = %#v", selected)
	}
}

func structConfig(address, username, password, database string) db.ClickHouseLogConfig {
	return db.ClickHouseLogConfig{
		Address: address, Username: username, Password: password, Database: database,
	}
}
