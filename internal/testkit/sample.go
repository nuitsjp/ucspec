// Package testkit exposes JSON fixtures to tests without production external dependencies.
package testkit

import (
	"github.com/nuitsjp/ucspec/internal/compiler"
	"github.com/nuitsjp/ucspec/internal/model"
	"path/filepath"
	"runtime"
	"testing"
)

func Sample(t testing.TB) *model.Spec {
	t.Helper()
	_, f, _, _ := runtime.Caller(0)
	p := filepath.Join(filepath.Dir(f), "../../examples/customer-crud/spec/project.json")
	s, e := compiler.Load(p, compiler.JSONCodec())
	if e != nil {
		t.Fatal(e)
	}
	return s
}
