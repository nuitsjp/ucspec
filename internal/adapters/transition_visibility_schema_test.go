package adapters

import (
	"encoding/json"
	"testing"

	"github.com/nuitsjp/ucspec/internal/assets"
)

func TestProductionSchemaTransitionDefaultVisibleBoundary(t *testing.T) {
	c, err := Codec(assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		raw   string
		valid bool
	}{
		{
			name:  "omitted",
			raw:   `{"schemaVersion":"1","transitions":{"open":{"from":"start","to":"done","event":"open","action":"Open"}}}`,
			valid: true,
		},
		{
			name:  "true",
			raw:   `{"schemaVersion":"1","transitions":{"open":{"from":"start","to":"done","event":"open","action":"Open","defaultVisible":true}}}`,
			valid: true,
		},
		{
			name:  "false",
			raw:   `{"schemaVersion":"1","transitions":{"open":{"from":"start","to":"done","event":"open","action":"Open","defaultVisible":false}}}`,
			valid: true,
		},
		{
			name:  "wrong type",
			raw:   `{"schemaVersion":"1","transitions":{"open":{"from":"start","to":"done","event":"open","action":"Open","defaultVisible":"true"}}}`,
			valid: false,
		},
		{
			name:  "null",
			raw:   `{"schemaVersion":"1","transitions":{"open":{"from":"start","to":"done","event":"open","action":"Open","defaultVisible":null}}}`,
			valid: false,
		},
		{
			name:  "unknown field remains rejected",
			raw:   `{"schemaVersion":"1","transitions":{"open":{"from":"start","to":"done","event":"open","action":"Open","defaultVisibility":true}}}`,
			valid: false,
		},
	}
	for _, tc := range tests {
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
