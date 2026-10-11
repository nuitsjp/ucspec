package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/model"
)

func packageValidationSpec() *model.Spec {
	s := &model.Spec{
		Document: model.EmptyDocument(),
		Project: model.Project{
			SchemaVersion: "1",
			Name:          "packages",
			Files:         []string{"model.json"},
			Output:        "artifacts",
			Initial:       []model.Snapshot{{State: "start"}},
		},
		Sources: map[string]model.Source{},
	}
	s.Project.Limits.Defaults()
	s.Screens["main"] = model.Screen{Label: "Main"}
	s.States["start"] = model.State{Label: "Start", Screen: "main", Events: []string{"finish"}}
	s.States["done"] = model.State{Label: "Done", Screen: "main", Events: []string{}, Terminal: true}
	s.Events["finish"] = model.Event{Label: "Finish", Source: "user"}
	s.Transitions["finish"] = model.Transition{From: "start", To: "done", Event: "finish", Action: "Finish"}
	s.UseCases["rootUseCase"] = model.UseCase{
		Name:      "Root use case",
		Goal:      "Finish",
		Actor:     "user",
		Starts:    []model.Snapshot{{State: "start"}},
		End:       map[string]any{"eq": []any{map[string]any{"var": "$state"}, "done"}},
		Scope:     []string{"finish"},
		Scenarios: map[string]model.Scenario{},
	}
	return s
}

func packageDiagnostic(ds []model.Diagnostic, subject, message string) bool {
	for _, d := range ds {
		if d.Subject == subject && strings.Contains(d.Message, message) && d.Severity == "error" {
			return true
		}
	}
	return false
}

func TestUseCasePackagesValidateNestedAndRootUseCase(t *testing.T) {
	s := packageValidationSpec()
	s.UseCasePackages["root"] = model.UseCasePackage{Name: "Root"}
	s.UseCasePackages["middle"] = model.UseCasePackage{Name: "Middle", Parent: "root"}
	s.UseCasePackages["leaf"] = model.UseCasePackage{Name: "Leaf", Parent: "middle"}
	s.UseCases["nestedUseCase"] = model.UseCase{
		Name:      "Nested use case",
		Goal:      "Finish",
		Actor:     "user",
		Package:   "leaf",
		Starts:    []model.Snapshot{{State: "start"}},
		End:       map[string]any{"eq": []any{map[string]any{"var": "$state"}, "done"}},
		Scope:     []string{"finish"},
		Scenarios: map[string]model.Scenario{},
	}
	if s.UseCases["rootUseCase"].Package != "" {
		t.Fatal("test fixture must leave the root use case package unspecified")
	}
	for _, d := range Validate(s) {
		if strings.HasPrefix(d.Subject, "usecasePackages/") || (strings.HasPrefix(d.Subject, "usecases/") && strings.Contains(d.Message, "package")) {
			t.Fatalf("valid package hierarchy rejected: %+v", d)
		}
	}
}

func TestUseCasePackagesValidateReferencesAndHierarchy(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		message string
		change  func(*model.Spec)
	}{
		{
			name:    "empty name",
			subject: "usecasePackages/root",
			message: "name required",
			change: func(s *model.Spec) {
				s.UseCasePackages["root"] = model.UseCasePackage{}
			},
		},
		{
			name:    "unknown parent",
			subject: "usecasePackages/root",
			message: "unknown parent package missing",
			change: func(s *model.Spec) {
				s.UseCasePackages["root"] = model.UseCasePackage{Name: "Root", Parent: "missing"}
			},
		},
		{
			name:    "self parent",
			subject: "usecasePackages/root",
			message: "package cannot be its own parent",
			change: func(s *model.Spec) {
				s.UseCasePackages["root"] = model.UseCasePackage{Name: "Root", Parent: "root"}
			},
		},
		{
			name:    "cycle",
			subject: "usecasePackages/root",
			message: "package hierarchy contains a cycle",
			change: func(s *model.Spec) {
				s.UseCasePackages["root"] = model.UseCasePackage{Name: "Root", Parent: "leaf"}
				s.UseCasePackages["middle"] = model.UseCasePackage{Name: "Middle", Parent: "root"}
				s.UseCasePackages["leaf"] = model.UseCasePackage{Name: "Leaf", Parent: "middle"}
			},
		},
		{
			name:    "unknown use case package",
			subject: "usecases/rootUseCase",
			message: "unknown use-case package missing",
			change: func(s *model.Spec) {
				s.UseCasePackages["root"] = model.UseCasePackage{Name: "Root"}
				u := s.UseCases["rootUseCase"]
				u.Package = "missing"
				s.UseCases["rootUseCase"] = u
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := packageValidationSpec()
			tc.change(s)
			if !packageDiagnostic(Validate(s), tc.subject, tc.message) {
				t.Fatalf("missing package diagnostic %s: %s", tc.subject, tc.message)
			}
		})
	}
}

func TestEmptyDocumentInitializesUseCasePackages(t *testing.T) {
	if d := model.EmptyDocument(); d.UseCasePackages == nil {
		t.Fatal("EmptyDocument did not initialize use case packages")
	}
}

func writePackageSource(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func packageProject(files []string) string {
	return model.JSON(model.Project{
		SchemaVersion: "1",
		Name:          "packages",
		Files:         files,
		Output:        "artifacts",
		Initial:       []model.Snapshot{{State: "start"}},
	})
}

func TestLoadMergesUseCasePackagesAndPreservesSources(t *testing.T) {
	root := t.TempDir()
	writePackageSource(t, root, "project.json", packageProject([]string{"packages.json", "usecases.json"}))
	writePackageSource(t, root, "packages.json", `{"schemaVersion":"1","usecasePackages":{"root":{"name":"Root"},"middle":{"name":"Middle","parent":"root"}}}`)
	writePackageSource(t, root, "usecases.json", `{"schemaVersion":"1","usecasePackages":{"leaf":{"name":"Leaf","parent":"middle"}},"usecases":{"rootUseCase":{"package":""},"nestedUseCase":{"package":"leaf"}}}`)

	s, err := Load(filepath.Join(root, "project.json"), JSONCodec())
	if err != nil {
		t.Fatalf("package documents did not load: %v", err)
	}
	if len(s.UseCasePackages) != 3 || s.UseCasePackages["leaf"].Parent != "middle" {
		t.Fatalf("merged package hierarchy is wrong: %#v", s.UseCasePackages)
	}
	if s.UseCases["nestedUseCase"].Package != "leaf" {
		t.Fatalf("merged use case package is wrong: %#v", s.UseCases["nestedUseCase"])
	}
	if got := s.Sources["usecasePackages/leaf"]; got.File != "usecases.json" || got.Pointer != "/usecasePackages/leaf" {
		t.Fatalf("unexpected package source pointer: %+v", got)
	}
	if got := s.Sources["usecases/nestedUseCase"]; got.File != "usecases.json" || got.Pointer != "/usecases/nestedUseCase" {
		t.Fatalf("unexpected use case source pointer: %+v", got)
	}
}

func TestLoadRejectsDuplicateUseCasePackages(t *testing.T) {
	root := t.TempDir()
	writePackageSource(t, root, "project.json", packageProject([]string{"one.json", "two.json"}))
	writePackageSource(t, root, "one.json", `{"schemaVersion":"1","usecasePackages":{"root":{"name":"Root"}}}`)
	writePackageSource(t, root, "two.json", `{"schemaVersion":"1","usecasePackages":{"root":{"name":"Other"}}}`)

	if _, err := Load(filepath.Join(root, "project.json"), JSONCodec()); err == nil || !strings.Contains(err.Error(), "duplicate usecasePackages/root") {
		t.Fatalf("duplicate package accepted or wrong error: %v", err)
	}
}
