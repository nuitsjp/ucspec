package adapters

import (
	"encoding/json"
	"testing"

	"github.com/nuitsjp/ucspec/internal/assets"
)

func packageSchemaDocument(t *testing.T, raw string) any {
	t.Helper()
	var doc any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestProductionCodecAcceptsNestedUseCasePackagesDocument(t *testing.T) {
	c, err := Codec(assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	valid := packageSchemaDocument(t, `{
		"schemaVersion":"1",
		"usecasePackages":{
			"root":{"name":"Root"},
			"middle":{"name":"Middle","parent":"root"},
			"leaf":{"name":"Leaf","parent":"middle"}
		}
	}`)
	if err := c.Validate("document", valid); err != nil {
		t.Fatalf("package-only document rejected by production schema: %v", err)
	}
}

func TestProductionCodecRejectsInvalidUseCasePackageFields(t *testing.T) {
	c, err := Codec(assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "package value has invalid type",
			raw:  `{"schemaVersion":"1","usecasePackages":[]}`,
		},
		{
			name: "package id has invalid format",
			raw:  `{"schemaVersion":"1","usecasePackages":{"bad id":{"name":"Bad"}}}`,
		},
		{
			name: "package name has invalid type",
			raw:  `{"schemaVersion":"1","usecasePackages":{"root":{"name":7}}}`,
		},
		{
			name: "package parent has invalid type",
			raw:  `{"schemaVersion":"1","usecasePackages":{"root":{"name":"Root","parent":7}}}`,
		},
		{
			name: "package has unknown field",
			raw:  `{"schemaVersion":"1","usecasePackages":{"root":{"name":"Root","extra":true}}}`,
		},
		{
			name: "use case package has invalid type",
			raw:  `{"schemaVersion":"1","usecases":{"uc":{"name":"Use case","goal":"Goal","actor":"Actor","package":7,"starts":[{"state":"start"}],"end":true,"scope":["finish"],"scenarios":{}}}}`,
		},
		{
			name: "use case package has invalid id",
			raw:  `{"schemaVersion":"1","usecases":{"uc":{"name":"Use case","goal":"Goal","actor":"Actor","package":"bad id","starts":[{"state":"start"}],"end":true,"scope":["finish"],"scenarios":{}}}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := c.Validate("document", packageSchemaDocument(t, tc.raw)); err == nil {
				t.Fatalf("production schema accepted malformed package field: %s", tc.raw)
			}
		})
	}
}
