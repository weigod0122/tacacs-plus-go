package clickhouse

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"tacacs/pkg/public/db"
	"tacacs/pkg/public/logquery"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

var (
	ErrNotConfigured = errors.New("clickhouse log store is not configured")
	identifierRE     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

type Column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Position int    `json:"position"`
}

type TableSchema struct {
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
}

type Filter struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type QueryRequest struct {
	Type         logquery.LogType
	From         time.Time
	To           time.Time
	Filters      []Filter
	Columns      []string
	Page         int
	PageSize     int
	IncludeTotal bool
}

type ResultColumn struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Physical string `json:"-"`
}

type QueryResult struct {
	Type       logquery.LogType `json:"type"`
	Columns    []ResultColumn   `json:"columns"`
	Rows       []map[string]any `json:"rows"`
	Page       int              `json:"page"`
	PageSize   int              `json:"pageSize"`
	Total      uint64           `json:"total"`
	TotalKnown bool             `json:"totalKnown"`
	TotalPages int              `json:"totalPages"`
	HasMore    bool             `json:"hasMore"`
}

type Manager struct {
	mu           sync.RWMutex
	conn         *sql.DB
	config       db.ClickHouseLogConfig
	schemaMu     sync.RWMutex
	schemas      map[logquery.LogType]TableSchema
	querySem     chan struct{}
	querySemOnce sync.Once
}

var DefaultManager = NewManager()

func NewManager() *Manager {
	return &Manager{
		schemas:  make(map[logquery.LogType]TableSchema),
		querySem: make(chan struct{}, 4),
	}
}

func (m *Manager) TestConnection(ctx context.Context, cfg db.ClickHouseLogConfig) error {
	conn, err := openConnection(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	var one int
	return conn.QueryRowContext(ctx, "SELECT 1").Scan(&one)
}

// ValidateConfig opens a temporary connection and validates all three table
// mappings without changing the active pool.
func (m *Manager) ValidateConfig(ctx context.Context, cfg db.ClickHouseLogConfig) error {
	conn, err := openConnection(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	return validateMappings(ctx, conn, cfg)
}

// Apply validates a complete configuration against ClickHouse before making
// it active. The previous pool remains usable if opening or validation fails.
func (m *Manager) Apply(ctx context.Context, cfg db.ClickHouseLogConfig) error {
	conn, err := openConnection(cfg)
	if err != nil {
		return err
	}
	schemas, err := validateMappingsWithSchemas(ctx, conn, cfg)
	if err != nil {
		_ = conn.Close()
		return err
	}

	m.mu.Lock()
	old := m.conn
	m.conn = conn
	m.config = cfg
	m.schemaMu.Lock()
	m.schemas = schemas
	m.schemaMu.Unlock()
	m.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}

func (m *Manager) Discover(ctx context.Context, cfg db.ClickHouseLogConfig) ([]TableSchema, error) {
	conn, err := openConnection(cfg)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return discoverWithConn(ctx, conn, cfg.Database)
}

func (m *Manager) Query(ctx context.Context, req QueryRequest) (QueryResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Page > 1_000_000 {
		return QueryResult{}, errors.New("page is too large")
	}
	if req.PageSize <= 0 {
		req.PageSize = 50
	}
	if req.PageSize > 200 {
		req.PageSize = 200
	}
	if req.Page > 100000 {
		return QueryResult{}, errors.New("page is too large")
	}
	if req.To.IsZero() || req.From.IsZero() || !req.From.Before(req.To) {
		return QueryResult{}, errors.New("event range must be non-empty and from must be before to")
	}
	if req.To.Sub(req.From) > 31*24*time.Hour {
		return QueryResult{}, errors.New("event range cannot exceed 31 days")
	}
	if len(req.Filters) > 20 {
		return QueryResult{}, errors.New("too many filters")
	}
	if len(req.Columns) > 64 {
		return QueryResult{}, errors.New("too many result columns")
	}
	// Keep Manager values constructed as literals safe for callers outside
	// this package and for older tests.
	m.querySemOnce.Do(func() { m.querySem = make(chan struct{}, 4) })
	select {
	case m.querySem <- struct{}{}:
		defer func() { <-m.querySem }()
	case <-ctx.Done():
		return QueryResult{}, ctx.Err()
	}

	if err := m.ensureLoaded(ctx); err != nil {
		return QueryResult{}, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.conn == nil {
		return QueryResult{}, ErrNotConfigured
	}
	cfg := m.config
	mapping := cfg.Mappings[req.Type]
	if !logquery.MappingComplete(req.Type, mapping) {
		return QueryResult{}, fmt.Errorf("log type %s is not fully mapped", req.Type)
	}

	m.schemaMu.RLock()
	schema, ok := m.schemas[req.Type]
	m.schemaMu.RUnlock()
	if !ok || schema.Name != mapping.Table {
		// A lazily loaded manager has no schema cache yet. Validate only the
		// requested stream here; Apply still validates all three streams when an
		// administrator saves a new configuration.
		var err error
		schema, err = tableSchema(ctx, m.conn, cfg.Database, mapping.Table)
		if err != nil {
			return QueryResult{}, err
		}
		if err := validateMappingSchema(req.Type, mapping, schema); err != nil {
			return QueryResult{}, err
		}
		m.schemaMu.Lock()
		if m.schemas == nil {
			m.schemas = make(map[logquery.LogType]TableSchema)
		}
		m.schemas[req.Type] = schema
		m.schemaMu.Unlock()
	}
	columnsByName := make(map[string]Column, len(schema.Columns))
	for _, column := range schema.Columns {
		columnsByName[column.Name] = column
	}
	eventColumn, ok := columnsByName[mapping.EventTimeColumn]
	if !ok {
		return QueryResult{}, fmt.Errorf("event time column %q no longer exists", mapping.EventTimeColumn)
	}

	selected, err := selectedColumns(req.Type, mapping, columnsByName, req.Columns)
	if err != nil {
		return QueryResult{}, err
	}
	prewhere, prewhereArgs, filterWhere, filterArgs, projection, err := buildQueryParts(req, mapping, columnsByName, eventColumn)
	if err != nil {
		return QueryResult{}, err
	}
	args := append(append([]any(nil), prewhereArgs...), filterArgs...)
	qualifiedTable := quoteIdentifier(cfg.Database) + "." + quoteIdentifier(mapping.Table)

	// Keep the event range in PREWHERE so MergeTree can discard granules
	// before loading the selected business columns. When the schema's
	// projection keys are filtered by equality, buildQueryParts also moves
	// those predicates into PREWHERE so ClickHouse can choose the matching
	// p_by_user_time / p_by_switch_time / p_by_switch_user_time projection.
	projectionSettings := ""
	if projection != "" {
		projectionSettings = " SETTINGS optimize_use_projections = 1"
	}
	countSQL := "SELECT count() FROM " + qualifiedTable + " PREWHERE " + prewhere
	if filterWhere != "" {
		countSQL += " WHERE " + filterWhere
	}
	countSQL += projectionSettings
	var total uint64

	offset := (req.Page - 1) * req.PageSize
	if offset < 0 || offset > 100000 {
		return QueryResult{}, errors.New("page is too large")
	}
	selectParts := make([]string, 0, len(selected))
	resultColumns := make([]ResultColumn, 0, len(selected))
	for _, column := range selected {
		selectParts = append(selectParts, quoteIdentifier(column.Physical))
		resultColumns = append(resultColumns, column)
	}
	querySQL := "SELECT " + strings.Join(selectParts, ", ") +
		" FROM " + qualifiedTable + " PREWHERE " + prewhere
	if filterWhere != "" {
		querySQL += " WHERE " + filterWhere
	}
	fetchLimit := req.PageSize
	if !req.IncludeTotal {
		// Exact counts require a second scan of the ClickHouse range. The normal
		// UI path only needs to know whether the next page exists, so fetch one
		// sentinel row instead of blocking on count(). Callers that need an exact
		// total can opt in with IncludeTotal.
		fetchLimit++
	}
	querySQL +=
		" ORDER BY " + quoteIdentifier(eventColumn.Name) + " DESC" +
			fmt.Sprintf(" LIMIT %d OFFSET %d", fetchLimit, offset)
	querySQL += projectionSettings

	// The exact count and the page scan touch the same range. Running them at
	// the same time removes their network/queue latency from the critical path
	// (and the pool has separate connections available for both operations).
	queryCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	var countErr, rowsErr error
	var rowsOut []map[string]any
	if req.IncludeTotal {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.conn.QueryRowContext(queryCtx, countSQL, args...).Scan(&total); err != nil {
				countErr = fmt.Errorf("count clickhouse log rows: %w", err)
				cancel()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		rows, err := m.conn.QueryContext(queryCtx, querySQL, args...)
		if err != nil {
			rowsErr = fmt.Errorf("query clickhouse log rows: %w", err)
			cancel()
			return
		}
		defer rows.Close()

		rowsOut = make([]map[string]any, 0, fetchLimit)
		for rows.Next() {
			values := make([]any, len(resultColumns))
			pointers := make([]any, len(values))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				rowsErr = fmt.Errorf("scan clickhouse log row: %w", err)
				cancel()
				return
			}
			row := make(map[string]any, len(values))
			for i, column := range resultColumns {
				row[column.Key] = normalizeValue(values[i])
			}
			rowsOut = append(rowsOut, row)
		}
		if err := rows.Err(); err != nil {
			rowsErr = fmt.Errorf("iterate clickhouse log rows: %w", err)
			cancel()
		}
	}()
	wg.Wait()
	if countErr != nil {
		return QueryResult{}, countErr
	}
	if rowsErr != nil {
		return QueryResult{}, rowsErr
	}

	hasMore := false
	if req.IncludeTotal {
		hasMore = uint64(req.Page)*uint64(req.PageSize) < total
	} else if len(rowsOut) > req.PageSize {
		hasMore = true
		rowsOut = rowsOut[:req.PageSize]
	}
	totalPages := 0
	if req.IncludeTotal && total > 0 {
		totalPages = int((total + uint64(req.PageSize) - 1) / uint64(req.PageSize))
	}
	return QueryResult{
		Type:       req.Type,
		Columns:    resultColumns,
		Rows:       rowsOut,
		Page:       req.Page,
		PageSize:   req.PageSize,
		Total:      total,
		TotalKnown: req.IncludeTotal,
		TotalPages: totalPages,
		HasMore:    hasMore,
	}, nil
}

func (m *Manager) ensureLoaded(ctx context.Context) error {
	m.mu.RLock()
	ready := m.conn != nil
	m.mu.RUnlock()
	if ready {
		return nil
	}
	cfg, err := db.GetClickHouseLogConfig()
	if err != nil {
		return err
	}
	if cfg.Address == "" || cfg.Database == "" {
		return ErrNotConfigured
	}
	conn, err := openConnection(cfg)
	if err != nil {
		return err
	}

	// Configuration is validated when it is saved. On a restart, avoid doing
	// three remote system.columns scans before the first page can be served;
	// Query validates and caches only the stream the caller actually requests.
	m.mu.Lock()
	if m.conn != nil {
		m.mu.Unlock()
		_ = conn.Close()
		return nil
	}
	m.conn = conn
	m.config = cfg
	m.schemaMu.Lock()
	m.schemas = make(map[logquery.LogType]TableSchema)
	m.schemaMu.Unlock()
	m.mu.Unlock()
	return nil
}

func openConnection(cfg db.ClickHouseLogConfig) (*sql.DB, error) {
	if strings.TrimSpace(cfg.Address) == "" {
		return nil, ErrNotConfigured
	}
	if !identifierRE.MatchString(strings.TrimSpace(cfg.Database)) {
		return nil, fmt.Errorf("invalid clickhouse database name")
	}
	dsn, err := buildDSN(cfg)
	if err != nil {
		return nil, err
	}
	conn, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(8)
	conn.SetMaxIdleConns(4)
	conn.SetConnMaxLifetime(30 * time.Minute)
	pingCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := conn.PingContext(pingCtx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func buildDSN(cfg db.ClickHouseLogConfig) (string, error) {
	address := strings.TrimSpace(cfg.Address)
	scheme := "clickhouse"
	host := address
	if strings.Contains(address, "://") {
		u, err := url.Parse(address)
		if err != nil || u.Host == "" {
			return "", fmt.Errorf("invalid clickhouse address")
		}
		switch u.Scheme {
		case "clickhouse", "http", "https":
			scheme = u.Scheme
		default:
			return "", fmt.Errorf("unsupported clickhouse address scheme %q", u.Scheme)
		}
		host = u.Host
	}
	if host == "" {
		return "", fmt.Errorf("clickhouse address is empty")
	}
	u := &url.URL{Scheme: scheme, Host: host, Path: "/" + cfg.Database}
	if cfg.Username != "" {
		u.User = url.UserPassword(cfg.Username, cfg.Password)
	}
	q := u.Query()
	q.Set("dial_timeout", "5s")
	q.Set("read_timeout", "30s")
	q.Set("max_execution_time", "30")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func discoverWithConn(ctx context.Context, conn *sql.DB, database string) ([]TableSchema, error) {
	rows, err := conn.QueryContext(ctx,
		"SELECT name FROM system.tables WHERE database = ? ORDER BY name", database)
	if err != nil {
		return nil, fmt.Errorf("list clickhouse tables: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if identifierRE.MatchString(name) {
			names = append(names, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(names) > 200 {
		names = names[:200]
	}
	result := make([]TableSchema, 0, len(names))
	for _, name := range names {
		schema, err := tableSchema(ctx, conn, database, name)
		if err != nil {
			return nil, err
		}
		if len(schema.Columns) > 200 {
			schema.Columns = schema.Columns[:200]
		}
		result = append(result, schema)
	}
	return result, nil
}

func tableSchema(ctx context.Context, conn *sql.DB, database, table string) (TableSchema, error) {
	if !identifierRE.MatchString(database) || !identifierRE.MatchString(table) {
		return TableSchema{}, errors.New("invalid clickhouse identifier")
	}
	rows, err := conn.QueryContext(ctx,
		"SELECT name, type, position FROM system.columns WHERE database = ? AND table = ? ORDER BY position",
		database, table)
	if err != nil {
		return TableSchema{}, fmt.Errorf("list columns for %s: %w", table, err)
	}
	defer rows.Close()
	schema := TableSchema{Name: table}
	for rows.Next() {
		var column Column
		if err := rows.Scan(&column.Name, &column.Type, &column.Position); err != nil {
			return TableSchema{}, err
		}
		if identifierRE.MatchString(column.Name) {
			schema.Columns = append(schema.Columns, column)
		}
	}
	if err := rows.Err(); err != nil {
		return TableSchema{}, err
	}
	return schema, nil
}

func validateMappings(ctx context.Context, conn *sql.DB, cfg db.ClickHouseLogConfig) error {
	_, err := validateMappingsWithSchemas(ctx, conn, cfg)
	return err
}

func validateMappingsWithSchemas(ctx context.Context, conn *sql.DB, cfg db.ClickHouseLogConfig) (map[logquery.LogType]TableSchema, error) {
	if cfg.Database == "" || !identifierRE.MatchString(cfg.Database) {
		return nil, errors.New("invalid clickhouse database")
	}
	schemas := make(map[logquery.LogType]TableSchema, len(logquery.AllLogTypes()))
	for _, typ := range logquery.AllLogTypes() {
		mapping := cfg.Mappings[typ]
		if !logquery.MappingComplete(typ, mapping) {
			return nil, fmt.Errorf("mapping for %s is incomplete or has duplicate columns", typ)
		}
		schema, err := tableSchema(ctx, conn, cfg.Database, mapping.Table)
		if err != nil {
			return nil, err
		}
		if err := validateMappingSchema(typ, mapping, schema); err != nil {
			return nil, err
		}
		schemas[typ] = schema
	}
	return schemas, nil
}

func validateMappingSchema(typ logquery.LogType, mapping logquery.Mapping, schema TableSchema) error {
	columns := make(map[string]Column, len(schema.Columns))
	for _, column := range schema.Columns {
		columns[column.Name] = column
	}
	if _, ok := columns[mapping.EventTimeColumn]; !ok {
		return fmt.Errorf("event time column %q is missing from %s", mapping.EventTimeColumn, mapping.Table)
	}
	for _, spec := range logquery.Specs(typ) {
		column, ok := columns[mapping.Fields[spec.Key]]
		if !ok {
			return fmt.Errorf("field %s maps to missing column %q", spec.Key, mapping.Fields[spec.Key])
		}
		if !compatible(spec.Kind, column.Type) {
			return fmt.Errorf("field %s (%s) is incompatible with ClickHouse type %s", spec.Key, spec.Kind, column.Type)
		}
	}
	return nil
}

func selectedColumns(typ logquery.LogType, mapping logquery.Mapping, physical map[string]Column, requested []string) ([]ResultColumn, error) {
	if len(requested) == 0 {
		for _, spec := range logquery.Specs(typ) {
			requested = append(requested, spec.Key)
		}
	}
	selected := make([]ResultColumn, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, key := range requested {
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate result column %q", key)
		}
		seen[key] = struct{}{}
		spec, ok := logquery.FieldSpecFor(typ, key)
		if !ok {
			return nil, fmt.Errorf("unknown result field %q", key)
		}
		physicalName := mapping.Fields[key]
		column, ok := physical[physicalName]
		if !ok {
			return nil, fmt.Errorf("result column %q no longer exists", physicalName)
		}
		selected = append(selected, ResultColumn{
			Key: spec.Key, Label: logquery.DisplayLabel(typ, spec.Key, mapping),
			Type: column.Type, Physical: column.Name,
		})
	}
	if len(selected) == 0 {
		return nil, errors.New("at least one result column is required")
	}
	return selected, nil
}

func buildWhere(req QueryRequest, mapping logquery.Mapping, columns map[string]Column, event Column) (string, []any, error) {
	eventWhere, eventArgs, filterWhere, filterArgs, err := buildWhereParts(req, mapping, columns, event)
	if err != nil {
		return "", nil, err
	}
	where := eventWhere
	args := append(append([]any(nil), eventArgs...), filterArgs...)
	if filterWhere != "" {
		where += " AND " + filterWhere
	}
	return where, args, nil
}

type filterClause struct {
	spec     logquery.FieldSpec
	column   Column
	operator string
	sql      string
	arg      any
}

// buildQueryParts keeps the mandatory event range in PREWHERE and promotes
// only the equality predicates that match the physical projection keys. The
// loghub schema has projections ordered by user, switchAddr, or both; other
// operators and fields stay in WHERE so a contains/range predicate cannot
// accidentally force an unsuitable access path.
func buildQueryParts(req QueryRequest, mapping logquery.Mapping, columns map[string]Column, event Column) (prewhere string, prewhereArgs []any, filterWhere string, filterArgs []any, projection string, err error) {
	eventWhere, eventArgs, err := eventWhereParts(req, event)
	if err != nil {
		return "", nil, "", nil, "", err
	}
	clauses, err := buildFilterClauses(req, mapping, columns)
	if err != nil {
		return "", nil, "", nil, "", err
	}
	projection, optimized := projectionFilterIndexes(mapping, event, clauses)

	preParts := []string{eventWhere}
	optimizedClauses := make([]filterClause, 0, len(optimized))
	for index := range clauses {
		if _, ok := optimized[index]; ok {
			optimizedClauses = append(optimizedClauses, clauses[index])
		}
	}
	preParts = append(preParts, clauseSQL(optimizedClauses)...)
	prewhere = strings.Join(preParts, " AND ")
	prewhereArgs = append(prewhereArgs, eventArgs...)
	prewhereArgs = append(prewhereArgs, clauseArgs(optimizedClauses)...)

	filterParts := make([]string, 0, len(clauses)-len(optimized))
	filterArgs = make([]any, 0, len(clauses)-len(optimized))
	for i := range clauses {
		if _, ok := optimized[i]; ok {
			continue
		}
		clause := &clauses[i]
		filterParts = append(filterParts, clause.sql)
		if clause.arg != nil {
			filterArgs = append(filterArgs, clause.arg)
		}
	}
	filterWhere = strings.Join(filterParts, " AND ")
	return prewhere, prewhereArgs, filterWhere, filterArgs, projection, nil
}

func buildWhereParts(req QueryRequest, mapping logquery.Mapping, columns map[string]Column, event Column) (string, []any, string, []any, error) {
	eventWhere, eventArgs, err := eventWhereParts(req, event)
	if err != nil {
		return "", nil, "", nil, err
	}
	clauses, err := buildFilterClauses(req, mapping, columns)
	if err != nil {
		return "", nil, "", nil, err
	}
	filterParts := clauseSQL(clauses)
	return eventWhere, eventArgs, strings.Join(filterParts, " AND "), clauseArgs(clauses), nil
}

func eventWhereParts(req QueryRequest, event Column) (string, []any, error) {
	if strings.TrimSpace(event.Name) == "" {
		return "", nil, errors.New("event time column is required")
	}
	return strings.Join([]string{
		fmt.Sprintf("%s >= ?", quoteIdentifier(event.Name)),
		fmt.Sprintf("%s < ?", quoteIdentifier(event.Name)),
	}, " AND "), []any{eventValue(event.Type, req.From), eventValue(event.Type, req.To)}, nil
}

func buildFilterClauses(req QueryRequest, mapping logquery.Mapping, columns map[string]Column) ([]filterClause, error) {
	clauses := make([]filterClause, 0, len(req.Filters))
	for _, filter := range req.Filters {
		if strings.TrimSpace(filter.Value) == "" {
			continue
		}
		spec, ok := logquery.FieldSpecFor(req.Type, filter.Field)
		if !ok {
			return nil, fmt.Errorf("unknown filter field %q", filter.Field)
		}
		physical := mapping.Fields[spec.Key]
		column, ok := columns[physical]
		if !ok {
			return nil, fmt.Errorf("filter column %q no longer exists", physical)
		}
		part, arg, err := filterSQL(spec, column, filter.Operator, filter.Value)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, filterClause{
			spec: spec, column: column, operator: strings.ToLower(strings.TrimSpace(filter.Operator)), sql: part, arg: arg,
		})
	}
	return clauses, nil
}

func clauseSQL(clauses []filterClause) []string {
	parts := make([]string, 0, len(clauses))
	for _, clause := range clauses {
		parts = append(parts, clause.sql)
	}
	return parts
}

func clauseArgs(clauses []filterClause) []any {
	args := make([]any, 0, len(clauses))
	for _, clause := range clauses {
		if clause.arg != nil {
			args = append(args, clause.arg)
		}
	}
	return args
}

func projectionFilterIndexes(mapping logquery.Mapping, event Column, clauses []filterClause) (string, map[int]struct{}) {
	if event.Name != "event_time" {
		return "", nil
	}
	hasUser := false
	hasSwitch := false
	for index := range clauses {
		clause := clauses[index]
		if clause.operator != "eq" {
			continue
		}
		switch clause.spec.Key {
		case "user":
			if clause.column.Name == "user" && mapping.Fields[clause.spec.Key] == "user" {
				hasUser = true
			}
		case "switchAddr":
			if clause.column.Name == "switchAddr" && mapping.Fields[clause.spec.Key] == "switchAddr" {
				hasSwitch = true
			}
		}
	}
	projection := ""
	switch {
	case hasUser && hasSwitch:
		projection = "p_by_switch_user_time"
	case hasUser:
		projection = "p_by_user_time"
	case hasSwitch:
		projection = "p_by_switch_time"
	}
	if projection == "" {
		return "", nil
	}
	optimized := make(map[int]struct{}, 2)
	for index := range clauses {
		clause := clauses[index]
		if clause.operator != "eq" {
			continue
		}
		if clause.spec.Key == "user" && hasUser && clause.column.Name == "user" {
			optimized[index] = struct{}{}
		}
		if clause.spec.Key == "switchAddr" && hasSwitch && clause.column.Name == "switchAddr" {
			optimized[index] = struct{}{}
		}
	}
	return projection, optimized
}

func filterSQL(spec logquery.FieldSpec, column Column, operator, raw string) (string, any, error) {
	identifier := quoteIdentifier(column.Name)
	operator = strings.ToLower(strings.TrimSpace(operator))
	baseType := normalizeType(column.Type)

	if spec.Kind == logquery.FieldKindStringArray {
		if operator != "contains" && operator != "not_contains" {
			return "", nil, fmt.Errorf("array field %s only supports contains/not_contains", spec.Key)
		}
		if !strings.HasPrefix(baseType, "Array(") {
			return "", nil, fmt.Errorf("field %s is not backed by an Array column", spec.Key)
		}
		if operator == "contains" {
			return "has(" + identifier + ", ?)", raw, nil
		}
		return "NOT has(" + identifier + ", ?)", raw, nil
	}

	var value any
	var err error
	if isDateType(baseType) {
		value, err = parseTimeValue(raw)
	} else {
		switch spec.Kind {
		case logquery.FieldKindInt:
			value, err = strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		case logquery.FieldKindBool:
			value, err = strconv.ParseBool(strings.TrimSpace(raw))
		default:
			value = raw
		}
	}
	if err != nil {
		return "", nil, fmt.Errorf("invalid value for %s: %w", spec.Key, err)
	}

	switch operator {
	case "eq":
		return identifier + " = ?", value, nil
	case "neq":
		return identifier + " != ?", value, nil
	case "gt":
		if spec.Kind != logquery.FieldKindInt {
			return "", nil, fmt.Errorf("gt is not supported for %s", spec.Key)
		}
		return identifier + " > ?", value, nil
	case "gte":
		if spec.Kind != logquery.FieldKindInt {
			return "", nil, fmt.Errorf("gte is not supported for %s", spec.Key)
		}
		return identifier + " >= ?", value, nil
	case "lt":
		if spec.Kind != logquery.FieldKindInt {
			return "", nil, fmt.Errorf("lt is not supported for %s", spec.Key)
		}
		return identifier + " < ?", value, nil
	case "lte":
		if spec.Kind != logquery.FieldKindInt {
			return "", nil, fmt.Errorf("lte is not supported for %s", spec.Key)
		}
		return identifier + " <= ?", value, nil
	case "contains", "not_contains", "starts_with", "ends_with":
		if spec.Kind != logquery.FieldKindString {
			return "", nil, fmt.Errorf("%s is not supported for %s", operator, spec.Key)
		}
		if operator == "contains" {
			return "positionCaseInsensitive(toString(" + identifier + "), ?) > 0", raw, nil
		}
		if operator == "not_contains" {
			return "positionCaseInsensitive(toString(" + identifier + "), ?) = 0", raw, nil
		}
		if operator == "starts_with" {
			return "startsWith(toString(" + identifier + "), ?)", raw, nil
		}
		return "endsWith(toString(" + identifier + "), ?)", raw, nil
	default:
		return "", nil, fmt.Errorf("unsupported filter operator %q", operator)
	}
}

func parseTimeValue(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, nil
	}
	return time.ParseInLocation("2006-01-02 15:04:05", raw, time.Local)
}

func eventValue(chType string, value time.Time) any {
	base := normalizeType(chType)
	if isDateType(base) {
		return value
	}
	if strings.HasPrefix(base, "Int") || strings.HasPrefix(base, "UInt") {
		return value.UnixNano()
	}
	return value.In(time.Local).Format("2006-01-02 15:04:05.000")
}

func compatible(kind logquery.FieldKind, chType string) bool {
	base := normalizeType(chType)
	switch kind {
	case logquery.FieldKindString:
		return base == "String" || strings.HasPrefix(base, "FixedString(") || isDateType(base)
	case logquery.FieldKindInt:
		return strings.HasPrefix(base, "Int") || strings.HasPrefix(base, "UInt") || strings.HasPrefix(base, "Decimal")
	case logquery.FieldKindBool:
		return base == "Bool" || base == "UInt8" || base == "Int8"
	case logquery.FieldKindStringArray:
		return strings.HasPrefix(base, "Array(") && strings.Contains(base, "String")
	default:
		return false
	}
}

func normalizeType(raw string) string {
	t := strings.TrimSpace(raw)
	for {
		switch {
		case strings.HasPrefix(t, "Nullable(") && strings.HasSuffix(t, ")"):
			t = strings.TrimSuffix(strings.TrimPrefix(t, "Nullable("), ")")
		case strings.HasPrefix(t, "LowCardinality(") && strings.HasSuffix(t, ")"):
			t = strings.TrimSuffix(strings.TrimPrefix(t, "LowCardinality("), ")")
		default:
			return t
		}
	}
}

func isDateType(t string) bool {
	return strings.HasPrefix(t, "Date")
}

func quoteIdentifier(identifier string) string {
	// All identifiers originate from system.columns or pass identifierRE before
	// reaching this function. Keeping the check here makes future call sites
	// fail closed instead of accidentally interpolating arbitrary SQL.
	if !identifierRE.MatchString(identifier) {
		return "``"
	}
	return "`" + identifier + "`"
}

func normalizeValue(value any) any {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return string(v)
	case time.Time:
		return v.Format(time.RFC3339Nano)
	case string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return value
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		return normalizeValue(rv.Elem().Interface())
	}
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		out := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = normalizeValue(rv.Index(i).Interface())
		}
		return out
	}
	return fmt.Sprint(value)
}
