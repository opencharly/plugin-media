package params

// decode_test.go — pins this generated projection against its SOURCE OF TRUTH, in both directions.
//
// charly hands a plugin its parameters as JSON (`spec.Op`'s params_json) and the plugin decodes
// that JSON into this projection; the sdk's kit.DecodeInput re-marshals to JSON before it
// unmarshals too. No YAML decoder ever sees these types — which is why the projection's `yaml:`
// tags are inert, and why this test pins the JSON side rather than asserting that claim in prose.
//
// The expected keys below are copied from the `#TranscodeInput` definition in
// `candy/plugin-media/schema/*.cue` — the schema that GENERATES this file. They are hardcoded on
// purpose: deriving them from the struct's own `json` tags (the obvious thing) makes the test
// vacuous, because a tag that drifts takes its derived expectation with it and the decode still
// lines up.

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// schemaKeys is the field set of #TranscodeInput, taken from candy/plugin-media/schema.
var schemaKeys = []string{
	"artifact",
	"artifact_min_bytes",
	"artifact_not_uniform",
	"source_artifact",
	"to",
}

func TestProjectionMatchesItsSchema(t *testing.T) {
	rt := reflect.TypeOf(TranscodeInput{})
	ptr := reflect.New(rt)

	// Direction 1: STRUCT -> SCHEMA. Every exported field must carry a json key the schema
	// declares; a drifted, emptied or removed tag is caught here.
	wire := map[string]any{}
	exported := 0
	tagOf := map[string]string{} // json key -> Go field name
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.PkgPath != "" {
			continue // unexported
		}
		exported++
		key := strings.Split(f.Tag.Get("json"), ",")[0]
		if key == "" || key == "-" {
			t.Fatalf("field %s carries no usable json tag (%q) — no schema key can reach it", f.Name, key)
		}
		if !contains(schemaKeys, key) {
			t.Fatalf("field %s is tagged json:%q, which #TranscodeInput in candy/plugin-media/schema does NOT declare — the projection has drifted from its source of truth", f.Name, key)
		}
		tagOf[key] = f.Name
		switch f.Type.Kind() {
		case reflect.String:
			wire[key] = "probe"
		case reflect.Bool:
			wire[key] = true
		case reflect.Int, reflect.Int64:
			wire[key] = 7
		case reflect.Float64:
			wire[key] = 1.5
		case reflect.Slice:
			wire[key] = []any{"probe"}
		case reflect.Map:
			wire[key] = map[string]any{"k": "v"}
		default:
			// nested structs/pointers/interfaces are not filled by a flat wire map
			continue
		}
	}
	if exported == 0 {
		t.Fatal("the projection has no exported fields — the test would prove nothing")
	}

	// Direction 2: SCHEMA -> STRUCT. Every key the schema declares must be carried by a field;
	// a key the projection dropped is caught here, and it is checked against schemaKeys directly
	// rather than against anything derived from the tags.
	for _, key := range schemaKeys {
		if _, ok := tagOf[key]; !ok {
			t.Fatalf("schema key %q is declared by #TranscodeInput but NO exported field carries it — the projection is missing part of its schema", key)
		}
	}

	b, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal the wire map: %v", err)
	}
	// THE PATH: json -> struct, exactly `json.Unmarshal(req.GetParamsJson(), &op)`.
	if err := json.Unmarshal(b, ptr.Interface()); err != nil {
		t.Fatalf("decode the wire map into the projection: %v", err)
	}
	elem := ptr.Elem()
	missing := []string{}
	for key := range wire {
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			if f.PkgPath == "" && strings.Split(f.Tag.Get("json"), ",")[0] == key && elem.Field(i).IsZero() {
				missing = append(missing, fmt.Sprintf("%s (field %s)", key, f.Name))
			}
		}
	}
	if len(missing) > 0 {
		t.Fatalf("%d wire key(s) were not delivered into the projection: %v", len(missing), missing)
	}
	t.Logf("both directions hold: %d exported field(s), %d schema key(s), %d decoded", exported, len(schemaKeys), len(wire))
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}
