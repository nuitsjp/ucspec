package report_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nuitsjp/ucspec/internal/analysis"
	"github.com/nuitsjp/ucspec/internal/assets"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/report"
	"github.com/nuitsjp/ucspec/internal/testkit"
	"strings"
	"testing"
	"testing/fstest"
)

func TestStandaloneHTMLAndEscaping(t *testing.T) {
	s := testkit.Sample(t)
	s.Project.Name = `<img src=x onerror=alert(1)>`
	u := s.UseCases["viewCustomer"]
	u.Goal = `</script><script>alert('x')</script>`
	s.UseCases["viewCustomer"] = u
	s.CommonRules["systemError"] = model.CommonRule{Name: `<img src=x onerror=alert(2)>`, Description: `</script><script>alert('rule')</script>`}
	r := analysis.Analyze(s, "test")
	b, e := report.Build(s, r, testkit.FixtureLayout, assets.Files)
	if e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{s.Project.Name, u.Goal, s.CommonRules["systemError"].Name, s.CommonRules["systemError"].Description, `<script src=`, `<link rel="stylesheet"`} {
		if bytes.Contains(b, []byte(bad)) {
			t.Fatalf("unsafe/nonselfcontained output %s", bad)
		}
	}
	if !bytes.Contains(b, []byte("sha256-")) || !bytes.Contains(b, []byte("data-transition")) {
		t.Fatal("CSP or graph missing")
	}
	if !bytes.Contains(b, []byte(`id="common-rules"`)) {
		t.Fatal("common rule section missing")
	}
	c, e := report.Build(s, r, testkit.FixtureLayout, assets.Files)
	if e != nil || !bytes.Equal(b, c) {
		t.Fatal("renderer is nondeterministic")
	}
}
func TestScriptCSPMatchesBrowserNewlines(t *testing.T) {
	const normalized = "'use strict';\nconsole.log('ready');\n"
	for _, newline := range []string{"\r\n", "\r"} {
		t.Run(fmt.Sprintf("%q", newline), func(t *testing.T) {
			files := fstest.MapFS{
				"viewer/style.css": {Data: []byte("body {}")},
				"viewer/app.js":    {Data: []byte(strings.ReplaceAll(normalized, "\n", newline))},
			}
			b, err := report.Build(nil, &model.Report{}, nil, files)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(b, []byte("<script>"+normalized+"</script>")) {
				t.Error("inline script is not normalized to browser LF newlines")
			}
			hash := sha256.Sum256([]byte(normalized))
			if !bytes.Contains(b, []byte("sha256-"+base64.StdEncoding.EncodeToString(hash[:]))) {
				t.Error("CSP hash does not match the script after browser newline normalization")
			}
		})
	}
}
func TestScenarioIntentAndCoverageReviewsArePreserved(t *testing.T) {
	s := testkit.Sample(t)
	goal := `<img src=x onerror=alert(1)>目的`
	reason := `</script><script>alert('review')</script>`
	r := &model.Report{
		Scenarios: []model.ScenarioResult{{ID: "viewCustomer/normal", Goal: goal, ExpectedOutcome: "顧客情報が確認できる"}},
		UseCases: []model.UseCaseResult{{
			ID: "viewCustomer", ReviewBasisHash: strings.Repeat("a", 64),
			Coverage: []model.Coverage{{Kind: "boundedPaths", Total: 2, Covered: 1, Missing: []model.Requirement{{ID: "retry > finish"}}}},
			CoverageReviews: []model.CoverageReviewResult{
				{ID: "retry", Status: "current", Review: model.CoverageReview{Kind: "boundedPaths", Requirements: []string{"retry > finish"}, Status: "represented", Reason: reason, Scenario: "normal", ArchitectureRequirements: []string{"保存処理を冪等にする"}, BasisHash: strings.Repeat("a", 64)}},
				{ID: "old", Status: "stale", Error: "review basis changed", Review: model.CoverageReview{Kind: "transitions", Requirements: []string{"removed"}, Status: "forbidden", Reason: "禁止操作"}},
			},
		}},
	}
	before := model.JSON(s) + model.JSON(r)
	b, err := report.Build(s, r, testkit.FixtureLayout, assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte(goal)) || bytes.Contains(b, []byte(reason)) {
		t.Fatal("scenario intent or review text injected into HTML")
	}
	_, data, ok := bytes.Cut(b, []byte(`<script id="spec-data" type="application/json">`))
	if !ok {
		t.Fatal("embedded data missing")
	}
	data, _, ok = bytes.Cut(data, []byte(`</script>`))
	if !ok {
		t.Fatal("embedded data is not terminated")
	}
	var payload struct {
		Spec   *model.Spec   `json:"spec"`
		Report *model.Report `json:"report"`
	}
	if err = json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if model.JSON(payload.Spec)+model.JSON(payload.Report) != before {
		t.Fatal("scenario intent, review records or raw coverage changed in embedded data")
	}
	if model.JSON(s)+model.JSON(r) != before {
		t.Fatal("renderer mutated scenario intent, review records or raw coverage")
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
