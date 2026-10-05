package logquery

import (
	"fmt"
	"sort"
	"strings"
)

// LogType identifies one of the three TACACS+ protocol log streams.
type LogType string

const (
	LogTypeAuthen  LogType = "authen"
	LogTypeAuthor  LogType = "author"
	LogTypeAccount LogType = "account"
)

func (t LogType) String() string { return string(t) }

func ParseLogType(raw string) (LogType, error) {
	t := LogType(raw)
	switch t {
	case LogTypeAuthen, LogTypeAuthor, LogTypeAccount:
		return t, nil
	default:
		return "", fmt.Errorf("unsupported log type %q", raw)
	}
}

// FieldKind describes the value shape expected by the corresponding TACACS+
// structure. It is also used to restrict the operators exposed by the UI.
type FieldKind string

const (
	FieldKindString      FieldKind = "string"
	FieldKindInt         FieldKind = "int"
	FieldKindBool        FieldKind = "bool"
	FieldKindStringArray FieldKind = "string_array"
)

type FieldSpec struct {
	Key   string    `json:"key"`
	Label string    `json:"label"`
	Kind  FieldKind `json:"kind"`
}

// Mapping stores the physical ClickHouse table/column mapping for one log
// stream. Fields and optional Names are keyed by the JSON field name used by
// the TACACS structs.
// StartTime is intentionally absent: it is an internal timing marker and is
// never emitted to the log hub or queried from ClickHouse.
type Mapping struct {
	Table           string            `json:"table"`
	EventTimeColumn string            `json:"eventTimeColumn"`
	Fields          map[string]string `json:"fields"`
	Names           map[string]string `json:"names,omitempty"`
}

// DisplayLabel returns the label used by the query metadata and result table.
// A configured name wins; older mappings without Names fall back to the
// structure field name (for example AuthenInfo.Time -> Time).
func DisplayLabel(t LogType, key string, mapping Mapping) string {
	if name := strings.TrimSpace(mapping.Names[key]); name != "" {
		return name
	}
	if spec, ok := FieldSpecFor(t, key); ok {
		if index := strings.LastIndex(spec.Label, "."); index >= 0 {
			return spec.Label[index+1:]
		}
		return spec.Label
	}
	return key
}

func Specs(t LogType) []FieldSpec {
	switch t {
	case LogTypeAuthen:
		return []FieldSpec{
			{Key: "time", Label: "AuthenInfo.Time", Kind: FieldKindString},
			{Key: "timeStamp", Label: "AuthenInfo.TimeStamp", Kind: FieldKindInt},
			{Key: "timeRange", Label: "AuthenInfo.TimeRange", Kind: FieldKindInt},
			{Key: "user", Label: "AuthenInfo.User", Kind: FieldKindString},
			{Key: "switchAddr", Label: "AuthenInfo.SwitchAddr", Kind: FieldKindString},
			{Key: "serverAddr", Label: "AuthenInfo.ServerAddr", Kind: FieldKindString},
			{Key: "authenStatus", Label: "AuthenInfo.AuthenStatus", Kind: FieldKindString},
			{Key: "details", Label: "AuthenInfo.Details", Kind: FieldKindString},
			{Key: "isSingleConnect", Label: "AuthenInfo.IsSingleConnect", Kind: FieldKindBool},
			{Key: "tacacsClient", Label: "AuthenInfo.TacacsClient", Kind: FieldKindString},
		}
	case LogTypeAuthor:
		return []FieldSpec{
			{Key: "time", Label: "AuthorInfo.Time", Kind: FieldKindString},
			{Key: "timeStamp", Label: "AuthorInfo.TimeStamp", Kind: FieldKindInt},
			{Key: "timeRange", Label: "AuthorInfo.TimeRange", Kind: FieldKindInt},
			{Key: "user", Label: "AuthorInfo.User", Kind: FieldKindString},
			{Key: "switchAddr", Label: "AuthorInfo.SwitchAddr", Kind: FieldKindString},
			{Key: "serverAddr", Label: "AuthorInfo.ServerAddr", Kind: FieldKindString},
			{Key: "authorStatus", Label: "AuthorInfo.AuthorStatus", Kind: FieldKindString},
			{Key: "details", Label: "AuthorInfo.Details", Kind: FieldKindString},
			{Key: "cmd", Label: "AuthorInfo.Cmd", Kind: FieldKindString},
			{Key: "isSingleConnect", Label: "AuthorInfo.IsSingleConnect", Kind: FieldKindBool},
			{Key: "tacacsClient", Label: "AuthorInfo.TacacsClient", Kind: FieldKindString},
		}
	case LogTypeAccount:
		return []FieldSpec{
			{Key: "time", Label: "AccountInfo.Time", Kind: FieldKindString},
			{Key: "timeStamp", Label: "AccountInfo.TimeStamp", Kind: FieldKindInt},
			{Key: "timeRange", Label: "AccountInfo.TimeRange", Kind: FieldKindInt},
			{Key: "user", Label: "AccountInfo.User", Kind: FieldKindString},
			{Key: "switchAddr", Label: "AccountInfo.SwitchAddr", Kind: FieldKindString},
			{Key: "serverAddr", Label: "AccountInfo.ServerAddr", Kind: FieldKindString},
			{Key: "cmd", Label: "AccountInfo.Cmd", Kind: FieldKindString},
			{Key: "port", Label: "AccountInfo.Port", Kind: FieldKindString},
			{Key: "flags", Label: "AccountInfo.Flags", Kind: FieldKindInt},
			{Key: "authenMethod", Label: "AccountInfo.AuthenMethod", Kind: FieldKindInt},
			{Key: "privLvl", Label: "AccountInfo.PrivLvl", Kind: FieldKindInt},
			{Key: "authenType", Label: "AccountInfo.AuthenType", Kind: FieldKindInt},
			{Key: "authenService", Label: "AccountInfo.AuthenService", Kind: FieldKindInt},
			{Key: "arg", Label: "AccountInfo.Arg", Kind: FieldKindStringArray},
			{Key: "isSingleConnect", Label: "AccountInfo.IsSingleConnect", Kind: FieldKindBool},
			{Key: "tacacsClient", Label: "AccountInfo.TacacsClient", Kind: FieldKindString},
		}
	default:
		return nil
	}
}

func FieldSpecFor(t LogType, key string) (FieldSpec, bool) {
	for _, spec := range Specs(t) {
		if spec.Key == key {
			return spec, true
		}
	}
	return FieldSpec{}, false
}

func AllLogTypes() []LogType {
	return []LogType{LogTypeAuthen, LogTypeAuthor, LogTypeAccount}
}

func MappingComplete(t LogType, m Mapping) bool {
	if m.Table == "" || m.EventTimeColumn == "" || len(m.Fields) != len(Specs(t)) {
		return false
	}
	seen := make(map[string]struct{}, len(m.Fields))
	for _, spec := range Specs(t) {
		column := m.Fields[spec.Key]
		if column == "" {
			return false
		}
		if _, ok := seen[column]; ok {
			return false
		}
		seen[column] = struct{}{}
	}
	return true
}

// MappingKeys returns the canonical keys in deterministic structure order.
// It is useful when a caller receives a map from JSON and needs stable output.
func MappingKeys(t LogType, m Mapping) []string {
	keys := make([]string, 0, len(Specs(t)))
	for _, spec := range Specs(t) {
		if _, ok := m.Fields[spec.Key]; ok {
			keys = append(keys, spec.Key)
		}
	}
	return keys
}

// SortedKeys is reserved for generic JSON maps returned by older clients.
func SortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
