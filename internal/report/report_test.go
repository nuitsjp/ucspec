package report_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/nuitsjp/ucspec/internal/analysis"
	"github.com/nuitsjp/ucspec/internal/assets"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/report"
	"github.com/nuitsjp/ucspec/internal/testkit"
	"io"
	"reflect"
	"regexp"
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
	svg := templateSVG(t, b, "diagram-system")
	for _, prefix := range []string{"ユーザー：", "システム：", "タイマー："} {
		if strings.Contains(svg, prefix) {
			t.Fatalf("redundant event source label: %s", prefix)
		}
	}
}

var startGroupTag = regexp.MustCompile(`<g\b[^>]*>`)
var startStateAttribute = regexp.MustCompile(`data-start-state="([^"]+)"`)
var transitionAttribute = regexp.MustCompile(`data-transition="([^"]+)"`)

func templateSVG(t *testing.T, page []byte, id string) string {
	t.Helper()
	text := string(page)
	open := `<template id="` + id + `">`
	start := strings.Index(text, open)
	if start < 0 {
		t.Fatalf("template %s missing", id)
	}
	start += len(open)
	end := strings.Index(text[start:], "</template>")
	if end < 0 {
		t.Fatalf("template %s is not closed", id)
	}
	return text[start : start+end]
}

func startStateCounts(svg, class string) map[string]int {
	counts := map[string]int{}
	for _, tag := range startGroupTag.FindAllString(svg, -1) {
		if !strings.Contains(tag, `class="`+class+`"`) {
			continue
		}
		match := startStateAttribute.FindStringSubmatch(tag)
		if len(match) == 2 {
			counts[match[1]]++
		}
	}
	return counts
}

func transitionCounts(svg string) map[string]int {
	counts := map[string]int{}
	for _, match := range transitionAttribute.FindAllStringSubmatch(svg, -1) {
		if len(match) == 2 {
			counts[match[1]]++
		}
	}
	return counts
}

func startMarkerFixture() (*model.Spec, *model.Report) {
	state := func(label string, events ...string) model.State {
		return model.State{Label: label, Screen: "main", Events: events}
	}
	s := &model.Spec{
		Project: model.Project{
			Name: "Start marker fixture",
			Initial: []model.Snapshot{
				{State: "sys-a", Values: map[string]any{"role": "editor"}},
				{State: "sys-a", Values: map[string]any{"role": "reader"}},
				{State: "sys-b", Values: map[string]any{"role": "reader"}},
			},
		},
		Document: model.Document{
			Screens: map[string]model.Screen{"main": {Label: "Main"}},
			States: map[string]model.State{
				"sys-a":   state("System A", "go"),
				"sys-b":   state("System B"),
				"uc-in":   state("Use case input", "go"),
				"uc-only": state("Use case only"),
				"end":     state("End"),
			},
			Events: map[string]model.Event{
				"go": {Label: "Go", Source: "user"},
			},
			Transitions: map[string]model.Transition{
				"system-transition": {From: "sys-a", To: "sys-b", Event: "go", Action: "system", DefaultVisible: true},
				"uc-transition":     {From: "uc-in", To: "end", Event: "go", Action: "use case", DefaultVisible: true},
			},
			UseCases: map[string]model.UseCase{
				"edit": {
					Name:  "Edit",
					Goal:  "Edit",
					Actor: "User",
					Starts: []model.Snapshot{
						{State: "uc-in", Values: map[string]any{"role": "editor"}},
						{State: "uc-in", Values: map[string]any{"role": "reader"}},
						{State: "uc-only", Values: map[string]any{"role": "reader"}},
					},
					Scope:     []string{"uc-transition"},
					Scenarios: map[string]model.Scenario{},
				},
			},
		},
	}
	r := &model.Report{Name: s.Project.Name, FormatVersion: "1", ToolVersion: "test", SchemaVersion: "1", Status: "verified"}
	return s, r
}

func TestStartMarkersUseDiagramSpecificStarts(t *testing.T) {
	s, r := startMarkerFixture()
	before := model.JSON(s) + model.JSON(r)
	b, err := report.Build(s, r, testkit.FixtureLayout, assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	system := templateSVG(t, b, "diagram-system")
	useCase := templateSVG(t, b, "diagram-uc-edit")
	wantSystem := map[string]int{"sys-a": 1, "sys-b": 1}
	wantUseCase := map[string]int{"uc-in": 1, "uc-only": 1}
	for _, class := range []string{"start-node", "start-edge"} {
		if got := startStateCounts(system, class); !reflect.DeepEqual(got, wantSystem) {
			t.Fatalf("system %s states: got %#v, want %#v", class, got, wantSystem)
		}
		if got := startStateCounts(useCase, class); !reflect.DeepEqual(got, wantUseCase) {
			t.Fatalf("use case %s states: got %#v, want %#v", class, got, wantUseCase)
		}
	}
	if got := transitionCounts(system); !reflect.DeepEqual(got, map[string]int{"system-transition": 1, "uc-transition": 1}) {
		t.Fatal("system business transition set changed")
	}
	if got := transitionCounts(useCase); !reflect.DeepEqual(got, map[string]int{"uc-transition": 1}) {
		t.Fatal("use case business transition set changed")
	}
	if before != model.JSON(s)+model.JSON(r) {
		t.Fatal("start markers mutated spec or report")
	}
}

func nestedFixture() (*model.Spec, *model.Report) {
	s, r := startMarkerFixture()
	s.States = map[string]model.State{
		"view":       {Label: "顧客閲覧", Screen: "main", Initial: "viewing"},
		"viewing":    {Label: "通常表示", Screen: "main", Parent: "view", Events: []string{"go"}},
		"confirming": {Label: "削除確認", Screen: "main", Parent: "view", Events: []string{"go"}},
		"deleting":   {Label: "削除中", Screen: "main", Parent: "view", Events: []string{"go"}},
		"end":        {Label: "完了", Screen: "main"},
	}
	s.Transitions = map[string]model.Transition{
		"open":    {From: "end", To: "view", Event: "go", DefaultVisible: true},
		"confirm": {From: "viewing", To: "confirming", Event: "go", DefaultVisible: true},
		"delete":  {From: "confirming", To: "deleting", Event: "go", DefaultVisible: true},
		"done":    {From: "deleting", To: "end", Event: "go", DefaultVisible: true},
		"leave":   {From: "view", To: "end", Event: "go", DefaultVisible: true},
	}
	s.Project.Initial = []model.Snapshot{{State: "view"}}
	s.UseCases["edit"] = model.UseCase{Name: "Delete", Starts: []model.Snapshot{{State: "deleting"}}, Scope: []string{"done"}, Scenarios: map[string]model.Scenario{}}
	return s, r
}

func svgStateParents(t *testing.T, svg string) map[string]string {
	t.Helper()
	parents := map[string]string{}
	stack := []string{}
	d := xml.NewDecoder(strings.NewReader(svg))
	for {
		token, err := d.Token()
		if err == io.EOF {
			return parents
		}
		if err != nil {
			t.Fatalf("invalid SVG: %v", err)
		}
		switch token := token.(type) {
		case xml.StartElement:
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			state := ""
			for _, a := range token.Attr {
				if a.Name.Local == "data-state" {
					state = a.Value
				}
			}
			if state != "" {
				parents[state] = parent
				parent = state
			}
			stack = append(stack, parent)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
}

func TestNestedStatesAndScopedInitialAreDrawnWithoutChangingSemantics(t *testing.T) {
	s, r := nestedFixture()
	before := model.JSON(s) + model.JSON(r)
	b, err := report.Build(s, r, testkit.FixtureLayout, assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	system := templateSVG(t, b, "diagram-system")
	uc := templateSVG(t, b, "diagram-uc-edit")
	if got := svgStateParents(t, system); !reflect.DeepEqual(got, map[string]string{"view": "", "viewing": "view", "confirming": "view", "deleting": "view", "end": ""}) {
		t.Fatalf("system hierarchy: %#v", got)
	}
	if got := svgStateParents(t, uc); !reflect.DeepEqual(got, map[string]string{"view": "", "viewing": "view", "deleting": "view", "end": ""}) {
		t.Fatalf("scoped ancestors and initial descendant: %#v", got)
	}
	for _, svg := range []string{system, uc} {
		if !strings.Contains(svg, `class="initial-node" data-parent-state="view" data-initial-state="viewing"`) || !strings.Contains(svg, `class="initial-edge" data-parent-state="view" data-initial-state="viewing"`) {
			t.Fatal("internal initial marker missing")
		}
	}
	if got := transitionCounts(uc); !reflect.DeepEqual(got, map[string]int{"done": 1}) {
		t.Fatalf("display-only initial became a business transition: %#v", got)
	}
	if before != model.JSON(s)+model.JSON(r) {
		t.Fatal("nested renderer changed model or report")
	}
}

func TestNestedEdgeContainerOffsetsAreAppliedToRoutesAndLabels(t *testing.T) {
	s, r := nestedFixture()
	layout := func(b []byte) ([]byte, error) {
		var g report.Graph
		if err := json.Unmarshal(b, &g); err != nil {
			return nil, err
		}
		g.Width, g.Height = 800, 600
		var locate func([]report.Shape)
		locate = func(nodes []report.Shape) {
			for i := range nodes {
				nodes[i].X, nodes[i].Y = 30, 40
				if nodes[i].ID == "view" {
					nodes[i].X, nodes[i].Y = 100, 50
					nodes[i].Width, nodes[i].Height = 600, 500
				}
				locate(nodes[i].Children)
				// Root connections may stay in the root array while their routes
				// and labels use the containing compound state's coordinates.
				for j := range nodes[i].Edges {
					nodes[i].Edges[j].Sections = []report.Section{{Start: report.Point{X: 10, Y: 20}, End: report.Point{X: 60, Y: 70}}}
				}
			}
		}
		locate(g.Children)
		for i := range g.Edges {
			g.Edges[i].Sections = []report.Section{{Start: report.Point{X: 10, Y: 20}, End: report.Point{X: 60, Y: 70}}}
			if g.Edges[i].ID == "confirm" {
				g.Edges[i].Container = "view"
				g.Edges[i].Labels = []report.Label{{Text: "Nested edge", X: 12, Y: 24, Width: 90, Height: 20}}
			}
		}
		return json.Marshal(g)
	}
	b, err := report.Build(s, r, layout, assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	svg := templateSVG(t, b, "diagram-system")
	if !strings.Contains(svg, `<g class="node composite" data-state="view" transform="translate(100.00 50.00)">`) || !strings.Contains(svg, `data-state="viewing" transform="translate(30.00 40.00)">`) {
		t.Fatal("parent-local node coordinates lost")
	}
	start := strings.Index(svg, `data-transition="confirm"`)
	if start < 0 {
		t.Fatal("nested confirmation edge missing")
	}
	end := strings.Index(svg[start:], `</g>`)
	edge := svg[start : start+end]
	if !strings.Contains(edge, `d="M110.00 70.00 L160.00 120.00 `) || !strings.Contains(edge, `<text x="112.00" y="86.00">Nested edge</text>`) {
		t.Fatalf("container offset not applied: %s", edge)
	}
	if !strings.Contains(svg, `<g class="initial-edge" data-parent-state="view" data-initial-state="viewing"><title>内部初期状態: 通常表示</title><path marker-end="url(#arrow)" d="M110.00 70.00 L160.00 120.00 `) {
		t.Fatal("nested edge array lost its owning compound offset")
	}
}
