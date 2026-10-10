package assets

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedDocumentationMatchesSource(t *testing.T) {
	for _, pair := range [][2]string{{"authoring.md", "../../docs/authoring.md"}, {"manual.md", "../../docs/user-manual.md"}} {
		a, e := Files.ReadFile(pair[0])
		if e != nil {
			t.Fatal(e)
		}
		b, e := os.ReadFile(pair[1])
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("guide out of sync: %s", pair[0])
		}
	}
}
func TestTemplateMatchesSampleSpecifications(t *testing.T) {
	e := fs.WalkDir(Files, "template/spec", func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		a, e := Files.ReadFile(path)
		if e != nil {
			return e
		}
		b, e := os.ReadFile(filepath.Join("../../examples/customer-crud", path[len("template/"):]))
		if e != nil {
			return e
		}
		if !bytes.Equal(a, b) {
			t.Errorf("template differs: %s", path)
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}
