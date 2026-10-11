package adapters

import (
	"encoding/json"
	"testing"

	"github.com/nuitsjp/ucspec/internal/assets"
)

func TestProductionSchemaStateLayoutLayerBoundary(t *testing.T) {
	c, err := Codec(assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	parse := func(raw string) any {
		t.Helper()
		var document any
		if err := json.Unmarshal([]byte(raw), &document); err != nil {
			t.Fatal(err)
		}
		return document
	}
	state := func(layer string) string {
		return `{"schemaVersion":"1","states":{"start":{"label":"Start","screen":"main","events":[]` + layer + `}},"screens":{"main":{"label":"Main"}}}`
	}
	for _, tc := range []struct {
		name  string
		layer string
		valid bool
	}{
		{name: "omitted", valid: true},
		{name: "zero", layer: `,"layoutLayer":0`, valid: true},
		{name: "positive", layer: `,"layoutLayer":7`, valid: true},
		{name: "negative", layer: `,"layoutLayer":-1`},
		{name: "non integer", layer: `,"layoutLayer":1.5`},
		{name: "null", layer: `,"layoutLayer":null`},
		{name: "string", layer: `,"layoutLayer":"1"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := c.Validate("document", parse(state(tc.layer))); (err == nil) != tc.valid {
				t.Fatalf("valid=%v, err=%v", tc.valid, err)
			}
		})
	}
}
