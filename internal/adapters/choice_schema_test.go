package adapters

import (
	"encoding/json"
	"testing"

	"github.com/nuitsjp/ucspec/internal/assets"
)

func TestProductionSchemaChoiceFields(t *testing.T) {
	c, err := Codec(assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		raw   string
		valid bool
	}{
		{`{"schemaVersion":"1","choices":{"result":{"label":"Result","parent":"workspace"}}}`, true},
		{`{"schemaVersion":"1","transitions":{"empty":{"from":"result","to":"ready","guard":true,"guardLabel":"No matches","action":"Alert"}}}`, true},
		{`{"schemaVersion":"1","decisions":{"branch":{"state":"result","cases":[{"enabled":["empty"]}]}}}`, true},
		{`{"schemaVersion":"1","commonRules":{"error":{"name":"System error","description":"Keep committed data."}}}`, true},
		{`{"schemaVersion":"1","commonRules":{"error":{"name":"System error"}}}`, false},
		{`{"schemaVersion":"1","commonRules":{"error":{"name":"","description":"Text"}}}`, false},
		{`{"schemaVersion":"1","commonRules":{"error":{"name":"Error","description":"Text","assert":true}}}`, false},
		{`{"schemaVersion":"1","choices":{"result":{"label":3}}}`, false},
		{`{"schemaVersion":"1","choices":{"result":{"label":"Result","parent":false}}}`, false},
		{`{"schemaVersion":"1","choices":{"result":{"label":"Result","events":[]}}}`, false},
		{`{"schemaVersion":"1","choices":{"bad id":{"label":"Result"}}}`, false},
		{`{"schemaVersion":"1","transitions":{"empty":{"from":"result","to":"ready","guard":true,"guardLabel":3,"action":"Alert"}}}`, false},
	}
	for _, test := range tests {
		var document any
		if err := json.Unmarshal([]byte(test.raw), &document); err != nil {
			t.Fatal(err)
		}
		if err := c.Validate("document", document); (err == nil) != test.valid {
			t.Fatalf("valid=%v, err=%v: %s", test.valid, err, test.raw)
		}
	}
}
