// Package app contains CLI orchestration. External adapters are supplied once by main.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/nuitsjp/usecase-spec/internal/analysis"
	"github.com/nuitsjp/usecase-spec/internal/compiler"
	"github.com/nuitsjp/usecase-spec/internal/model"
	"github.com/nuitsjp/usecase-spec/internal/project"
	"github.com/nuitsjp/usecase-spec/internal/report"
	"github.com/nuitsjp/usecase-spec/internal/update"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Dependencies struct {
	Codec      compiler.Codec
	Layout     report.Layout
	Assets     fs.FS
	Version    string
	Repository string
	Executable func() (string, error)
	Update     *update.Client
}

func Run(ctx context.Context, args []string, out, errOut io.Writer, d Dependencies) int {
	if d.Executable == nil {
		d.Executable = os.Executable
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(out, usage)
		return 0
	}
	if args[0] == "--version" {
		args = []string{"version"}
	}
	switch args[0] {
	case "version":
		f := flags("version", errOut)
		asJSON := f.Bool("json", false, "machine-readable version")
		if e := parse(f, args[1:]); e != nil {
			return flagExit(e)
		}
		if *asJSON {
			_ = json.NewEncoder(out).Encode(map[string]string{"version": d.Version, "schemaVersion": "1", "repository": d.Repository, "platform": runtime.GOOS + "/" + runtime.GOARCH})
		} else {
			fmt.Fprintf(out, "ucspec %s (%s/%s)\n", d.Version, runtime.GOOS, runtime.GOARCH)
		}
		return 0
	case "schema":
		f := flags("schema", errOut)
		path := f.String("out", "", "output file (default stdout)")
		if e := parse(f, args[1:]); e != nil {
			return flagExit(e)
		}
		b, e := fs.ReadFile(d.Assets, "schemas/spec.schema.json")
		if e != nil {
			return fail(errOut, e)
		}
		if *path == "" {
			_, e = out.Write(b)
		} else {
			e = atomicWrite(*path, b)
		}
		if e != nil {
			return fail(errOut, e)
		}
		return 0
	case "init":
		return initialize(args[1:], out, errOut, d)
	case "check", "build":
		return inspect(args[0], args[1:], out, errOut, d)
	case "self-update":
		return selfUpdate(ctx, args[1:], out, errOut, d)
	default:
		return fail(errOut, fmt.Errorf("unknown command %q; use ucspec help", args[0]))
	}
}
func flags(name string, out io.Writer) *flag.FlagSet {
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	f.SetOutput(out)
	return f
}
func parse(f *flag.FlagSet, args []string) error {
	if e := f.Parse(args); e != nil {
		return e
	}
	if f.NArg() != 0 {
		e := fmt.Errorf("unexpected positional arguments: %v", f.Args())
		fmt.Fprintln(f.Output(), e)
		return e
	}
	return nil
}
func flagExit(e error) int {
	if errors.Is(e, flag.ErrHelp) {
		return 0
	}
	return 2
}
func fail(w io.Writer, e error) int { fmt.Fprintln(w, "error:", e); return 2 }
func initialize(args []string, out, errOut io.Writer, d Dependencies) int {
	f := flags("init", errOut)
	dir := f.String("dir", ".", "product directory")
	name := f.String("name", "", "product name")
	repo := f.String("repo", d.Repository, "GitHub release repository owner/name")
	dry := f.Bool("dry-run", false, "list changes only")
	noTool := f.Bool("no-tool", false, "do not copy the running binary (source development)")
	if e := parse(f, args); e != nil {
		return flagExit(e)
	}
	exe := ""
	var e error
	if !*noTool {
		exe, e = d.Executable()
		if e != nil {
			return fail(errOut, e)
		}
	}
	items, e := project.Init(d.Assets, project.Options{Root: *dir, Name: *name, Version: d.Version, Repository: *repo, Executable: exe, DryRun: *dry})
	conflicts := false
	for _, it := range items {
		fmt.Fprintf(out, "%-10s %s\n", it.Action, it.Path)
		if it.Action == "conflict" {
			conflicts = true
		}
	}
	if e != nil {
		return fail(errOut, e)
	}
	if conflicts {
		return 1
	}
	if !*dry {
		fmt.Fprintln(out, "Initialized. Review spec/AGENTS.md; then run scripts/ucspec.ps1 build.")
	}
	return 0
}
func inspect(command string, args []string, out, errOut io.Writer, d Dependencies) int {
	f := flags(command, errOut)
	path := f.String("project", "spec/project.json", "project file (.json/.yaml)")
	outFlag := f.String("out", "", "output directory (explicit override)")
	baseline := f.String("baseline", "", "previous report.json")
	ignorePin := f.Bool("ignore-pin", false, "explicit development override; do not use in normal CI")
	if e := parse(f, args); e != nil {
		return flagExit(e)
	}
	absolute, e := filepath.Abs(*path)
	if e != nil {
		return fail(errOut, e)
	}
	root := filepath.Dir(filepath.Dir(absolute))
	if !*ignorePin {
		pin, e := os.ReadFile(filepath.Join(root, ".ucspec-version"))
		if e == nil && strings.TrimSpace(string(pin)) != d.Version {
			return fail(errOut, fmt.Errorf("tool version %s differs from project pin %s; use scripts/ucspec.ps1", d.Version, strings.TrimSpace(string(pin))))
		}
		if e != nil && !os.IsNotExist(e) {
			return fail(errOut, e)
		}
	}
	s, compileErr := compiler.Load(absolute, d.Codec)
	output := filepath.Join(root, "artifacts", "usecase-spec")
	if s != nil {
		if filepath.IsAbs(s.Project.Output) {
			return fail(errOut, fmt.Errorf("configured output must be relative to product root"))
		}
		output = filepath.Join(root, filepath.FromSlash(s.Project.Output))
		if e = project.SafePath(root, output); e != nil {
			return fail(errOut, e)
		}
		rel, _ := filepath.Rel(root, output)
		if rel == "." || rel == "spec" || strings.HasPrefix(filepath.ToSlash(rel), "spec/") {
			return fail(errOut, fmt.Errorf("output must not be product root or spec directory"))
		}
	}
	if *outFlag != "" {
		output, e = filepath.Abs(*outFlag)
		if e != nil {
			return fail(errOut, e)
		}
	}
	var r *model.Report
	if compileErr != nil {
		r = &model.Report{FormatVersion: "1", ToolVersion: d.Version, Name: "Specification load failed", Status: "failed", Complete: false, Diagnostics: []model.Diagnostic{{Code: "load", Severity: "error", Message: compileErr.Error(), Source: model.Source{File: *path}}}}
	} else {
		r = analysis.Analyze(s, d.Version)
	}
	if *baseline != "" && compileErr == nil {
		b, e := os.ReadFile(*baseline)
		if e != nil {
			return fail(errOut, e)
		}
		var prev model.Report
		if e = json.Unmarshal(b, &prev); e != nil {
			return fail(errOut, e)
		}
		if e = analysis.Compare(r, &prev); e != nil {
			return fail(errOut, e)
		}
	}
	var page []byte
	var renderErr error
	if command == "build" {
		if compileErr != nil || hasStructureErrors(r) {
			page = report.ErrorPage(model.JSON(r.Diagnostics))
		} else {
			page, renderErr = report.Build(s, r, d.Layout, d.Assets)
			if renderErr != nil {
				r.Status = "failed"
				r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "layout", Severity: "error", Message: renderErr.Error()})
				page = report.ErrorPage(renderErr.Error())
			}
		}
	}
	if e = os.MkdirAll(output, 0755); e != nil {
		return fail(errOut, e)
	}
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return fail(errOut, e)
	}
	if e = atomicWrite(filepath.Join(output, "report.json"), append(b, '\n')); e != nil {
		return fail(errOut, e)
	}
	if page != nil {
		if e = atomicWrite(filepath.Join(output, "index.html"), page); e != nil {
			return fail(errOut, e)
		}
	}
	fmt.Fprintf(out, "%s: %d reachable configurations, %d scenarios, %d diagnostics\n", r.Status, r.ReachableStates, len(r.Scenarios), len(r.Diagnostics))
	for _, u := range r.UseCases {
		for _, c := range u.Coverage {
			if c.Percent != nil {
				fmt.Fprintf(out, "  %s %-18s %d/%d (%.1f%%) complete=%t\n", u.ID, c.Kind, c.Covered, c.Total, *c.Percent, c.Complete)
			}
		}
	}
	fmt.Fprintln(out, "Report:", filepath.Join(output, "report.json"))
	if page != nil {
		fmt.Fprintln(out, "HTML:", filepath.Join(output, "index.html"))
	}
	if compileErr != nil || renderErr != nil {
		return 2
	}
	if r.Status != "verified" {
		return 1
	}
	return 0
}
func hasStructureErrors(r *model.Report) bool {
	for _, v := range r.Diagnostics {
		if v.Code == "structure" && v.Severity == "error" {
			return true
		}
	}
	return false
}
func atomicWrite(path string, data []byte) error {
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".ucspec-write-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	_, e = f.Write(data)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if e = os.Chmod(name, 0644); e != nil {
		return e
	}
	return os.Rename(name, path)
}
func selfUpdate(ctx context.Context, args []string, out, errOut io.Writer, d Dependencies) int {
	f := flags("self-update", errOut)
	repo := f.String("repo", d.Repository, "release repository owner/name")
	version := f.String("version", "", "stable version (default latest stable)")
	check := f.Bool("check", false, "show update without modifying files")
	projectDir := f.String("project", "", "install side-by-side and update this product's pin")
	downgrade := f.Bool("allow-downgrade", false, "explicitly allow installing an older version")
	if e := parse(f, args); e != nil {
		return flagExit(e)
	}
	current := d.Version
	if *projectDir != "" {
		b, e := os.ReadFile(filepath.Join(*projectDir, ".ucspec-version"))
		if e != nil {
			return fail(errOut, e)
		}
		current = strings.TrimSpace(string(b))
	}
	client := d.Update
	if client == nil {
		token := os.Getenv("GH_TOKEN")
		if token == "" {
			token = os.Getenv("GITHUB_TOKEN")
		}
		client = update.NewClient(token)
	}
	p, e := client.Resolve(ctx, *repo, *version)
	if e != nil {
		return fail(errOut, e)
	}
	cmp, e := update.CompareVersion(p.Version, current)
	if e != nil {
		return fail(errOut, e)
	}
	fmt.Fprintf(out, "Current: %s; release: %s; repository: %s\n", current, p.Version, p.Repository)
	if *check {
		return 0
	}
	if cmp == 0 {
		fmt.Fprintln(out, "Already at requested version.")
		return 0
	}
	if cmp < 0 && !*downgrade {
		return fail(errOut, fmt.Errorf("downgrade requires --allow-downgrade"))
	}
	if e = update.RequireWindows(); e != nil {
		return fail(errOut, e)
	}
	target, e := d.Executable()
	if e != nil {
		return fail(errOut, e)
	}
	if *projectDir == "" && update.IsProjectExecutable(target) {
		return fail(errOut, fmt.Errorf("this is a pinned project executable; use self-update --project <product-directory>"))
	}
	b, e := client.Download(ctx, p)
	if e != nil {
		return fail(errOut, e)
	}
	if e = probe(ctx, b, p.Version); e != nil {
		return fail(errOut, e)
	}
	if *projectDir != "" {
		path, e := update.InstallProject(*projectDir, p.Version, b)
		if e != nil {
			return fail(errOut, e)
		}
		fmt.Fprintln(out, "Project pin updated; installed:", path)
		return 0
	}
	backup, e := update.Replace(target, b)
	if e != nil {
		return fail(errOut, e)
	}
	fmt.Fprintln(out, "Updated. The NEXT invocation runs the new version. Previous executable:", backup)
	fmt.Fprintln(out, "Project .ucspec-version files were not modified.")
	return 0
}
func probe(ctx context.Context, data []byte, expected string) error {
	dir, e := os.MkdirTemp("", "ucspec-probe-*")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "ucspec.exe")
	if e = os.WriteFile(path, data, 0700); e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	b, e := exec.CommandContext(ctx, path, "version", "--json").Output()
	if e != nil {
		return fmt.Errorf("new executable failed version probe: %w", e)
	}
	var v struct {
		Version string `json:"version"`
		Schema  string `json:"schemaVersion"`
	}
	if e = json.Unmarshal(b, &v); e != nil {
		return e
	}
	if v.Version != expected || v.Schema != "1" {
		return fmt.Errorf("new executable reports unexpected version/schema")
	}
	return nil
}

const usage = `UCSPEC — structured use-case specifications and finite-model verification

Commands:
  ucspec init --dir PRODUCT --name NAME [--repo OWNER/REPO] [--dry-run]
  ucspec check [--project spec/project.json] [--baseline old/report.json]
  ucspec build [--project spec/project.json] [--out DIR]
  ucspec schema [--out spec/ucspec.schema.json]
  ucspec version [--json]
  ucspec self-update --check [--repo OWNER/REPO]
  ucspec self-update [--version X.Y.Z] [--repo OWNER/REPO]
  ucspec self-update --project PRODUCT [--version X.Y.Z]

Exit codes: 0 accepted; 1 verification/coverage failed or incomplete; 2 usage/I/O/build failure.
Only Windows amd64 binaries are distributed. No background update or telemetry.
Use '<command> --help' for flags. See README.md, docs/onboarding.md and docs/user-manual.md.
`
