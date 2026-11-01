package params

// decode_test.go — the ONE runtime path these generated structs participate in, pinned.
//
// charly hands a plugin its parameters as JSON (`spec.Op`'s params_json) and the plugin decodes
// that JSON into this projection; the sdk's kit.DecodeInput re-marshals to JSON before it
// unmarshals too. NO YAML decoder ever sees these types — that is why the projection's `yaml:`
// tags are inert, and why this test pins the JSON side rather than asserting the claim in prose.
//
// It fails if a json tag drifts away from the wire key: the field it belongs to would silently
// stop receiving its configured value, which is the failure mode a generated projection can have.

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestProjectionJSONDecodePopulatesTaggedFields(t *testing.T) {
	rt := reflect.TypeOf(TranscodeInput{})

	wire := map[string]any{}
	fillable := map[int]string{} // field index -> json key the probe can populate
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.PkgPath != "" {
			continue // unexported
		}
		key := strings.Split(f.Tag.Get("json"), ",")[0]
		if key == "" || key == "-" {
			continue
		}
		var v any
		switch f.Type.Kind() {
		case reflect.String:
			v = "probe"
		case reflect.Bool:
			v = true
		case reflect.Int, reflect.Int64:
			v = 7
		case reflect.Float64:
			v = 1.5
		case reflect.Slice:
			v = []any{"probe"}
		case reflect.Map:
			v = map[string]any{"k": "v"}
		default:
			// structs/pointers/interfaces are not filled by a flat wire map; skip them rather than
			// assert something this probe cannot exercise.
			continue
		}
		wire[key] = v
		fillable[i] = key
	}
	if len(fillable) == 0 {
		t.Fatal("no json-tagged field the probe can populate — the test would prove nothing")
	}

	b, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal the wire map: %v", err)
	}
	var got TranscodeInput
	// THE PATH: json -> struct, exactly `json.Unmarshal(req.GetParamsJson(), &op)`.
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("decode the wire map into the projection: %v", err)
	}

	rv := reflect.ValueOf(got)
	missing := []string{}
	for i, key := range fillable {
		if rv.Field(i).IsZero() {
			missing = append(missing, fmt.Sprintf("%s (field %s)", key, rt.Field(i).Name))
		}
	}
	if len(missing) > 0 {
		t.Fatalf("%d of %d json-tagged fields were NOT populated by the decode: %v",
			len(missing), len(fillable), missing)
	}
	t.Logf("decoded %d/%d json-tagged fields from the wire map", len(fillable), len(fillable))
}
