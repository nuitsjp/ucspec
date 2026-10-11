package adapters

import (
	"encoding/json"
	"testing"

	"github.com/nuitsjp/ucspec/internal/assets"
)

func TestProductionSchemaRequiresActionButAllowsEmptyAction(t *testing.T) {
	c, err := Codec(assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		raw   string
		valid bool
	}{
		{
			name:  "empty action",
			raw:   `{"schemaVersion":"1","transitions":{"advance":{"from":"start","to":"done","event":"advance","action":""}}}`,
			valid: true,
		},
		{
			name:  "missing action",
			raw:   `{"schemaVersion":"1","transitions":{"advance":{"from":"start","to":"done","event":"advance"}}}`,
			valid: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var document any
			if err := json.Unmarshal([]byte(tc.raw), &document); err != nil {
				t.Fatal(err)
			}
			if err := c.Validate("document", document); (err == nil) != tc.valid {
				t.Fatalf("valid=%v, err=%v: %s", tc.valid, err, tc.raw)
			}
		})
	}
}
