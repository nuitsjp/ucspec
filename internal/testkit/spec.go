package testkit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/nuitsjp/ucspec/internal/compiler"
	"github.com/nuitsjp/ucspec/internal/model"
)

// Project returns a minimal valid project for in-memory specifications.
func Project() model.Project {
	return model.Project{SchemaVersion: "1", Name: "test", Files: []string{"*.json"}, Output: "out", Limits: model.Limits{Steps: 10, Paths: 100000}, Coverage: map[string]float64{}}
}

// Compile decodes JSON documents strictly and compiles them as one specification.
func Compile(t testing.TB, docs ...string) *model.Spec {
	t.Helper()
	s, err := CompileError(docs...)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// CompileError is Compile without failing the test on a load error.
func CompileError(docs ...string) (*model.Spec, error) {
	decoded := map[string]model.Document{}
	for i, text := range docs {
		var doc model.Document
		d := json.NewDecoder(bytes.NewReader([]byte(text)))
		d.DisallowUnknownFields()
		if err := d.Decode(&doc); err != nil {
			return nil, fmt.Errorf("document %d: %w", i, err)
		}
		decoded[fmt.Sprintf("doc%02d.json", i)] = doc
	}
	return compiler.FromDocuments(Project(), decoded, nil)
}
