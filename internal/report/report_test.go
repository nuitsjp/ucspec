package report_test

import (
	"bytes"
	"errors"
	"github.com/nuitsjp/usecase-spec/internal/analysis"
	"github.com/nuitsjp/usecase-spec/internal/assets"
	"github.com/nuitsjp/usecase-spec/internal/model"
	"github.com/nuitsjp/usecase-spec/internal/report"
	"github.com/nuitsjp/usecase-spec/internal/testkit"
	"strings"
	"testing"
)

func TestStandaloneHTMLAndEscaping(t *testing.T) {
	s := testkit.Sample(t)
	s.Project.Name = `<img src=x onerror=alert(1)>`
	u := s.UseCases["viewCustomer"]
	u.Goal = `</script><script>alert('x')</script>`
	s.UseCases["viewCustomer"] = u
	r := analysis.Analyze(s, "test")
	b, e := report.Build(s, r, testkit.FixtureLayout, assets.Files)
	if e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{s.Project.Name, u.Goal, `<script src=`, `<link rel="stylesheet"`} {
		if bytes.Contains(b, []byte(bad)) {
			t.Fatalf("unsafe/nonselfcontained output %s", bad)
		}
	}
	if !bytes.Contains(b, []byte("sha256-")) || !bytes.Contains(b, []byte("data-transition")) {
		t.Fatal("CSP or graph missing")
	}
	c, e := report.Build(s, r, testkit.FixtureLayout, assets.Files)
	if e != nil || !bytes.Equal(b, c) {
		t.Fatal("renderer is nondeterministic")
	}
}
func TestRendererRejectsBrokenLayout(t *testing.T) {
	s := testkit.Sample(t)
	r := analysis.Analyze(s, "test")
	for _, fn := range []report.Layout{func([]byte) ([]byte, error) { return nil, errors.New("layout failure") }, func([]byte) ([]byte, error) { return []byte(`{"id":"root","width":-1,"height":-1}`), nil }} {
		if _, e := report.Build(s, r, fn, assets.Files); e == nil {
			t.Fatal("invalid layout accepted")
		}
	}
}
func TestErrorPageEscapes(t *testing.T) {
	s := `<script>alert(1)</script>`
	b := report.ErrorPage(s)
	if strings.Contains(string(b), s) {
		t.Fatal("raw error injected")
	}
	if len(b) == 0 {
		t.Fatal("empty error page")
	}
}
func TestModelAndReportRemainUnmodified(t *testing.T) {
	s := testkit.Sample(t)
	r := analysis.Analyze(s, "test")
	before := model.JSON(s) + model.JSON(r)
	if _, e := report.Build(s, r, testkit.FixtureLayout, assets.Files); e != nil {
		t.Fatal(e)
	}
	if before != model.JSON(s)+model.JSON(r) {
		t.Fatal("view mutated semantics")
	}
}

func TestLongJapaneseLabelsAndReservedLookingIDs(t *testing.T) {
	s := testkit.Sample(t)
	st := s.States["view"]
	st.Label = strings.Repeat("顧客情報の閲覧と確認", 10)
	s.States["view"] = st
	s.UseCases["all"] = s.UseCases["viewCustomer"]
	r := analysis.Analyze(s, "test")
	b, e := report.Build(s, r, testkit.FixtureLayout, assets.Files)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(b, []byte(`id="diagram-system"`)) || !bytes.Contains(b, []byte(`id="diagram-uc-all"`)) {
		t.Fatal("global/usecase diagram ID collision")
	}
	if !bytes.Contains(b, []byte("ユーザー")) {
		t.Fatal("event source absent")
	}
}
