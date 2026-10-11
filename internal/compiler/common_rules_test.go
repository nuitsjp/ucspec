package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/model"
)

func TestCommonRulesLoadValidateAndRejectDuplicates(t *testing.T) {
	s := packageValidationSpec()
	s.Project.Files = []string{"model.json", "common.json"}
	dir := t.TempDir()
	write := func(name string, value any) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(model.JSON(value)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("project.json", s.Project)
	write("model.json", s.Document)
	common := model.EmptyDocument()
	common.CommonRules["systemError"] = model.CommonRule{Name: "System error", Description: "Keep committed data and report the error."}
	write("common.json", common)
	loaded, err := Load(filepath.Join(dir, "project.json"), Codec{})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CommonRules["systemError"] != common.CommonRules["systemError"] || loaded.Sources["commonRules/systemError"].Pointer != "/commonRules/systemError" {
		t.Fatal("common rule or source missing")
	}
	if ds := Validate(loaded); len(ds) != 0 {
		t.Fatal(ds)
	}
	for _, rule := range []model.CommonRule{{Name: "", Description: "Body"}, {Name: "Name", Description: " "}} {
		loaded.CommonRules["systemError"] = rule
		if !packageDiagnostic(Validate(loaded), "commonRules/systemError", "name and description") {
			t.Fatal("empty common rule accepted")
		}
	}
	s.Project.Files = append(s.Project.Files, "duplicate.json")
	write("project.json", s.Project)
	write("duplicate.json", common)
	if _, err := Load(filepath.Join(dir, "project.json"), Codec{}); err == nil || !strings.Contains(err.Error(), "duplicate commonRules/systemError") {
		t.Fatalf("duplicate rule was accepted: %v", err)
	}
}
