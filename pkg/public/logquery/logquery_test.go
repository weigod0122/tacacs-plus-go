package logquery

import "testing"

func TestSpecsExcludeStartTimeAndMappingIsOneToOne(t *testing.T) {
	for _, typ := range AllLogTypes() {
		specs := Specs(typ)
		if len(specs) == 0 {
			t.Fatalf("%s has no fields", typ)
		}
		mapping := Mapping{Table: "logs", EventTimeColumn: "event_time", Fields: make(map[string]string, len(specs))}
		for _, spec := range specs {
			if spec.Key == "startTime" || spec.Label == "StartTime" {
				t.Fatalf("%s unexpectedly exposes StartTime", typ)
			}
			mapping.Fields[spec.Key] = "ck_" + spec.Key
		}
		if !MappingComplete(typ, mapping) {
			t.Fatalf("%s complete mapping rejected", typ)
		}
		mapping.Fields[specs[0].Key] = mapping.Fields[specs[1].Key]
		if MappingComplete(typ, mapping) {
			t.Fatalf("%s duplicate physical column accepted", typ)
		}
	}
}

func TestFieldSpecForRejectsUnknownField(t *testing.T) {
	if _, ok := FieldSpecFor(LogTypeAuthen, "doesNotExist"); ok {
		t.Fatal("unknown field was accepted")
	}
	if spec, ok := FieldSpecFor(LogTypeAccount, "arg"); !ok || spec.Kind != FieldKindStringArray {
		t.Fatalf("arg spec = %#v, %v", spec, ok)
	}
}

func TestDisplayLabelUsesConfiguredNameOrBareFieldName(t *testing.T) {
	mapping := Mapping{Names: map[string]string{"time": "发生时间"}}
	if got := DisplayLabel(LogTypeAuthen, "time", mapping); got != "发生时间" {
		t.Fatalf("configured label = %q", got)
	}
	if got := DisplayLabel(LogTypeAccount, "time", Mapping{}); got != "Time" {
		t.Fatalf("default label = %q", got)
	}
	if got := DisplayLabel(LogTypeAccount, "doesNotExist", Mapping{}); got != "doesNotExist" {
		t.Fatalf("unknown label = %q", got)
	}
}
