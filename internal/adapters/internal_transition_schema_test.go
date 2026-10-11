package adapters

import (
	"encoding/json"
	"testing"

	"github.com/nuitsjp/ucspec/internal/assets"
)

func TestProductionSchemaTransitionKind(t *testing.T) {
	c, err := Codec(assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		kind  string
		valid bool
	}{
		{name: "omitted", valid: true},
		{name: "external", kind: "external", valid: true},
		{name: "internal", kind: "internal", valid: true},
		{name: "unknown", kind: "loop", valid: false},
		{name: "number", kind: "1", valid: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"schemaVersion":"1","transitions":{"move":{"from":"start","to":"start","action":"Move"` +
				func() string {
					if tc.name == "omitted" {
						return ""
					}
					return `,"kind":"` + tc.kind + `"`
				}() + `}}}`
			var document any
			if err := json.Unmarshal([]byte(raw), &document); err != nil {
				t.Fatal(err)
			}
			if err := c.Validate("document", document); (err == nil) != tc.valid {
				t.Fatalf("valid=%v, err=%v: %s", tc.valid, err, raw)
			}
		})
	}

	for _, raw := range []string{
		`{"schemaVersion":"1","transitions":{"move":{"to":"start","action":"Move"}}}`,
		`{"schemaVersion":"1","transitions":{"move":{"from":"start","action":"Move"}}}`,
	} {
		var document any
		if err := json.Unmarshal([]byte(raw), &document); err != nil {
			t.Fatal(err)
		}
		if err := c.Validate("document", document); err == nil {
			t.Fatalf("schema accepted transition without required from/to: %s", raw)
		}
	}
}
