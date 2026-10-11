package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStateLayoutLayerJSONDistinguishesOmittedAndZero(t *testing.T) {
	zero := 0
	positive := 3
	withZero := State{LayoutLayer: &zero}
	withPositive := State{LayoutLayer: &positive}
	without := State{}

	zeroJSON, err := json.Marshal(withZero)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(zeroJSON), `"layoutLayer":0`) {
		t.Fatalf("zero layout layer was omitted: %s", zeroJSON)
	}
	positiveJSON, err := json.Marshal(withPositive)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(positiveJSON), `"layoutLayer":3`) {
		t.Fatalf("positive layout layer was omitted: %s", positiveJSON)
	}
	omittedJSON, err := json.Marshal(without)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(omittedJSON), `"layoutLayer"`) {
		t.Fatalf("nil layout layer was serialized: %s", omittedJSON)
	}

	var decodedZero State
	if err := json.Unmarshal(zeroJSON, &decodedZero); err != nil {
		t.Fatal(err)
	}
	if decodedZero.LayoutLayer == nil || *decodedZero.LayoutLayer != 0 {
		t.Fatalf("decoded zero layout layer = %v, want non-nil pointer to 0", decodedZero.LayoutLayer)
	}
	var decodedOmitted State
	if err := json.Unmarshal(omittedJSON, &decodedOmitted); err != nil {
		t.Fatal(err)
	}
	if decodedOmitted.LayoutLayer != nil {
		t.Fatalf("decoded omitted layout layer = %v, want nil", decodedOmitted.LayoutLayer)
	}
}
