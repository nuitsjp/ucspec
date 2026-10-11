package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nuitsjp/ucspec/internal/assets"
	"github.com/nuitsjp/ucspec/internal/compiler"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/project"
	"github.com/nuitsjp/ucspec/internal/testkit"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dependencies() Dependencies {
	return Dependencies{Codec: compiler.JSONCodec(), Layout: testkit.FixtureLayout, Assets: assets.Files, Version: "0.1.0", Repository: "nuitsjp/ucspec"}
}
func run(t *testing.T, d Dependencies, args ...string) (int, string) {
	t.Helper()
	var b bytes.Buffer
	code := Run(context.Background(), args, &b, &b, d)
	return code, b.String()
}
func initialized(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	_, e := project.Init(assets.Files, project.Options{Root: root, Name: "CLI Test", Version: "0.1.0", Repository: "nuitsjp/ucspec"})
	if e != nil {
		t.Fatal(e)
	}
	return root
}
func TestHelpVersionAndUnknownCommand(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"version"}, {"version", "--json"}, {"schema"}} {
		code, out := run(t, dependencies(), args...)
		if code != 0 || out == "" {
			t.Fatal(args, code, out)
		}
	}
	if code, _ := run(t, dependencies(), "wat"); code != 2 {
		t.Fatal(code)
	}
}
func TestCheckBuildAndPin(t *testing.T) {
	root := initialized(t)
	p := filepath.Join(root, "spec/project.json")
	for _, cmd := range []string{"check", "build"} {
		code, out := run(t, dependencies(), cmd, "--project", p)
		if code != 0 {
			t.Fatal(code, out)
		}
	}
	dir := filepath.Join(root, "artifacts/ucspec")
	for _, f := range []string{"index.html", "report.json"} {
		if _, e := os.Stat(filepath.Join(dir, f)); e != nil {
			t.Fatal(e)
		}
	}
	os.WriteFile(filepath.Join(root, ".ucspec-version"), []byte("1.0.0\n"), 0600)
	if code, _ := run(t, dependencies(), "check", "--project", p); code != 2 {
		t.Fatal("pin mismatch accepted")
	}
	if code, out := run(t, dependencies(), "check", "--project", p, "--ignore-pin"); code != 0 {
		t.Fatal(code, out)
	}
}
func TestBadInputAndLayoutCannotLeaveSuccessfulHTML(t *testing.T) {
	root := initialized(t)
	p := filepath.Join(root, "spec/project.json")
	d := dependencies()
	code, out := run(t, d, "build", "--project", p)
	if code != 0 {
		t.Fatal(out)
	}
	d.Layout = func([]byte) ([]byte, error) { return nil, errors.New("ELK FAIL") }
	code, out = run(t, d, "build", "--project", p)
	if code != 2 {
		t.Fatal(code, out)
	}
	b, _ := os.ReadFile(filepath.Join(root, "artifacts/ucspec/index.html"))
	if !strings.Contains(string(b), "ELK FAIL") {
		t.Fatal("stale HTML retained")
	}
	os.WriteFile(p, []byte("{invalid"), 0600)
	if code, _ = run(t, dependencies(), "check", "--project", p); code != 2 {
		t.Fatal(code)
	}
}

func TestSourceLoadFailureReplacesConfiguredOutput(t *testing.T) {
	for _, withBaseline := range []bool{false, true} {
		t.Run(fmt.Sprintf("baseline=%t", withBaseline), func(t *testing.T) {
			root := initialized(t)
			p := filepath.Join(root, "spec/project.json")
			cfg, err := compiler.ReadProject(p, compiler.JSONCodec())
			if err != nil {
				t.Fatal(err)
			}
			cfg.Output = "custom-report"
			if err = os.WriteFile(p, []byte(model.JSON(cfg)), 0600); err != nil {
				t.Fatal(err)
			}
			if code, out := run(t, dependencies(), "build", "--project", p); code != 0 {
				t.Fatal(code, out)
			}
			output := filepath.Join(root, cfg.Output)
			args := []string{"build", "--project", p}
			if withBaseline {
				b, err := os.ReadFile(filepath.Join(output, "report.json"))
				if err != nil {
					t.Fatal(err)
				}
				baseline := filepath.Join(root, "before.json")
				if err = os.WriteFile(baseline, b, 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--baseline", baseline)
			}
			if err = os.WriteFile(filepath.Join(root, "spec/model/customer.json"), []byte("{invalid"), 0600); err != nil {
				t.Fatal(err)
			}
			if code, out := run(t, dependencies(), args...); code != 2 {
				t.Fatal(code, out)
			}
			b, err := os.ReadFile(filepath.Join(output, "report.json"))
			if err != nil {
				t.Fatal(err)
			}
			var result model.Report
			if err = json.Unmarshal(b, &result); err != nil {
				t.Fatal(err)
			}
			if result.Status != "failed" || result.Complete || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "load" {
				t.Errorf("stale successful report retained: status=%s complete=%t diagnostics=%v", result.Status, result.Complete, result.Diagnostics)
			}
			b, err = os.ReadFile(filepath.Join(output, "index.html"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(b, []byte("仕様書生成に失敗しました")) {
				t.Error("stale successful HTML retained")
			}
			if _, err = os.Stat(filepath.Join(root, "artifacts/ucspec")); !os.IsNotExist(err) {
				t.Errorf("failure output redirected to default directory: %v", err)
			}
		})
	}
}
func TestInitCommandNoOverwrite(t *testing.T) {
	root := t.TempDir()
	args := []string{"init", "--dir", root, "--no-tool", "--name", "App"}
	if code, out := run(t, dependencies(), args...); code != 0 {
		t.Fatal(code, out)
	}
	os.WriteFile(filepath.Join(root, "spec/AGENTS.md"), []byte("local change"), 0600)
	if code, _ := run(t, dependencies(), args...); code != 2 {
		t.Fatal(code)
	}
}
func TestCoverageFailureExitCode(t *testing.T) {
	root := initialized(t)
	p := filepath.Join(root, "spec/project.json")
	cfg, e := compiler.ReadProject(p, compiler.JSONCodec())
	if e != nil {
		t.Fatal(e)
	}
	// Repeated searches still leave transition pairs uncovered in the sample.
	cfg.Coverage["pairs"] = 100
	os.WriteFile(p, []byte(model.JSON(cfg)), 0600)
	if code, out := run(t, dependencies(), "check", "--project", p); code != 1 {
		t.Fatal(code, out)
	}
}
func TestBaselineProducesImpact(t *testing.T) {
	root := initialized(t)
	p := filepath.Join(root, "spec/project.json")
	if code, out := run(t, dependencies(), "check", "--project", p); code != 0 {
		t.Fatal(out)
	}
	report := filepath.Join(root, "artifacts/ucspec/report.json")
	b, _ := os.ReadFile(report)
	baseline := filepath.Join(root, "before.json")
	os.WriteFile(baseline, b, 0600)
	if code, out := run(t, dependencies(), "build", "--project", p, "--baseline", baseline); code != 0 {
		t.Fatal(code, out)
	}
	b, _ = os.ReadFile(report)
	if !bytes.Contains(b, []byte(`"impact"`)) {
		t.Fatal("impact missing")
	}
}
