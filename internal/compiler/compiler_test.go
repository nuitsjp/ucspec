package compiler

import (
	"github.com/nuitsjp/usecase-spec/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestRejectDuplicateAndTrailingJSON(t *testing.T) {
	for _, x := range []string{`{"a":1,"a":2}`, `{"a":{"b":1,"b":2}}`, `{} {}`, `[1`} {
		if err := rejectDuplicateKeys([]byte(x)); err == nil {
			t.Errorf("accepted %s", x)
		}
	}
	if err := rejectDuplicateKeys([]byte(`{"a":[{"b":1},{"b":2}]}`)); err != nil {
		t.Fatal(err)
	}
}
func TestUnknownFields(t *testing.T) {
	var p model.Project
	err := decode(JSONCodec(), []byte(`{"schemaVersion":"1","typo":true}`), "x.json", "project", &p)
	if err == nil {
		t.Fatal("unknown field accepted")
	}
}
func TestCompilerFileEscape(t *testing.T) {
	d := t.TempDir()
	p := model.Project{SchemaVersion: "1", Name: "test", Files: []string{"../outside.json"}}
	os.WriteFile(filepath.Join(d, "project.json"), []byte(model.JSON(p)), 0600)
	if _, err := Load(filepath.Join(d, "project.json"), JSONCodec()); err == nil {
		t.Fatal("escape accepted")
	}
}
func TestMergeDoesNotOverwrite(t *testing.T) {
	s := &model.Spec{Document: model.EmptyDocument(), Sources: map[string]model.Source{}}
	d := model.EmptyDocument()
	d.States["a"] = model.State{Label: "A"}
	if err := merge(s, d, "a.json"); err != nil {
		t.Fatal(err)
	}
	if err := merge(s, d, "b.json"); err == nil {
		t.Fatal("duplicate accepted")
	}
	if s.Sources["states/a"].File != "a.json" {
		t.Fatal("source overwritten")
	}
}
func TestRequiredReferences(t *testing.T) {
	s := &model.Spec{Document: model.EmptyDocument(), Project: model.Project{SchemaVersion: "1", Name: "x", Initial: []model.Snapshot{{State: "missing"}}}}
	s.Transitions["broken"] = model.Transition{From: "a", To: "b", Event: "e", Action: "act"}
	ds := Validate(s)
	errors := 0
	for _, d := range ds {
		if d.Severity == "error" {
			errors++
		}
	}
	if errors == 0 {
		t.Fatal("invalid graph accepted")
	}
}

func TestCompoundPredicateCannotHideCoverageAtoms(t *testing.T) {
	x := map[string]any{"not": map[string]any{"and": []any{true, false}}}
	if atomicShape(x) {
		t.Fatal("hidden composite accepted")
	}
	if !atomicShape(map[string]any{"eq": []any{map[string]any{"var": "role"}, "editor"}}) {
		t.Fatal("ordinary predicate rejected")
	}
}
