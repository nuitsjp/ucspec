package project

import (
	"bytes"
	"github.com/nuitsjp/ucspec/internal/assets"
	"github.com/nuitsjp/ucspec/internal/compiler"
	"os"
	"path/filepath"
	"testing"
)

func options(root string) Options {
	return Options{Root: root, Name: "Test product", Version: "0.1.0", Repository: "nuitsjp/ucspec"}
}
func TestInitAndIdempotence(t *testing.T) {
	root := t.TempDir()
	o := options(root)
	items, e := Init(assets.Files, o)
	if e != nil || len(items) < 10 {
		t.Fatal(items, e)
	}
	p, e := compiler.Load(filepath.Join(root, "spec/project.json"), compiler.JSONCodec())
	if e != nil || p.Project.Name != o.Name {
		t.Fatal(e)
	}
	items, e = Init(assets.Files, o)
	if e != nil {
		t.Fatal(e)
	}
	for _, it := range items {
		if it.Action != "unchanged" {
			t.Fatal(it.Path, it.Action)
		}
	}
}
func TestInitConflictIsPreflighted(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "spec"), 0755)
	file := filepath.Join(root, "spec/project.json")
	os.WriteFile(file, []byte("keep"), 0600)
	if _, e := Init(assets.Files, options(root)); e == nil {
		t.Fatal("conflict accepted")
	}
	b, _ := os.ReadFile(file)
	if string(b) != "keep" {
		t.Fatal("user content replaced")
	}
	if _, e := os.Stat(filepath.Join(root, ".ucspec-version")); !os.IsNotExist(e) {
		t.Fatal("partial initialization")
	}
}
func TestInitDryRunDoesNotWrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "new")
	o := options(root)
	o.DryRun = true
	if _, e := Init(assets.Files, o); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(root); !os.IsNotExist(e) {
		t.Fatal("dry run wrote files")
	}
}
func TestInitCopiesBinaryAndPreservesRootGitignore(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "tool.exe")
	os.WriteFile(src, []byte("binary"), 0700)
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("original"), 0600)
	o := options(root)
	o.Executable = src
	if _, e := Init(assets.Files, o); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(root, ".tools/ucspec/0.1.0/ucspec.exe"))
	if string(b) != "binary" {
		t.Fatal("not copied")
	}
	b, _ = os.ReadFile(filepath.Join(root, ".gitignore"))
	if string(b) != "original" {
		t.Fatal("root gitignore modified")
	}
}
func TestInitDistributesAuthoringSkill(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(assets.Files, options(root)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, ".agents/skills/ucspec-authoring/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := assets.Files.ReadFile("ucspec-authoring-skill.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("installed authoring skill differs from embedded content")
	}
}
func TestInitPreservesExistingAuthoringSkill(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agents/skills/ucspec-authoring/SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	want := []byte("Product-specific authoring instructions\n")
	if err := os.WriteFile(path, want, 0644); err != nil {
		t.Fatal(err)
	}
	o := options(root)
	o.DryRun = true
	items, err := Init(assets.Files, o)
	if err != nil {
		t.Fatal(err)
	}
	conflictFound := false
	for _, item := range items {
		if item.Path == ".agents/skills/ucspec-authoring/SKILL.md" && item.Action == "conflict" {
			conflictFound = true
		}
	}
	if !conflictFound {
		t.Fatal("dry run did not report authoring skill conflict")
	}
	o.DryRun = false
	if _, err := Init(assets.Files, o); err == nil {
		t.Fatal("existing authoring skill was accepted")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("product authoring skill was replaced")
	}
	for _, rel := range []string{".ucspec-version", "spec/project.json"} {
		if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Fatalf("initialization left a partial file: %s", rel)
		}
	}
}
func TestSafePath(t *testing.T) {
	root := t.TempDir()
	if SafePath(root, filepath.Join(root, "../escape")) == nil {
		t.Fatal("escape accepted")
	}
	outside := t.TempDir()
	link := filepath.Join(root, "linked")
	if os.Symlink(outside, link) == nil {
		if SafePath(root, filepath.Join(link, "file")) == nil {
			t.Fatal("symlink followed")
		}
	}
}
