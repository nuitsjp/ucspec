package compiler_test

import (
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/compiler"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/testkit"
)

const nested = `{"schemaVersion":"1","start":{"to":"A"},"actors":{"人":{}},
 "events":{"終了":{"actors":["人"],"to":"A"}},
 "states":{"A":{"start":{"to":"A1"},"states":{
  "A1":{"events":{"次":{"actors":["人"],"to":"A2"},
                  "判定":{"actors":["人"],"action":"確認する","decision":{"label":"結果","branches":{"可":{"to":"A2"},"否":{}}}}}},
  "A2":{}}}},
 "usecases":{"u":{"name":"U","goal":"G","actor":"人","starts":["A1"],"ends":["A1:次"],"scope":["A1:次"],"scenarios":{}}}}`

func TestFlattenPathsAndTransitionIDs(t *testing.T) {
	s := testkit.Compile(t, nested)
	if d := compiler.Validate(s); len(d) != 0 {
		t.Fatal(d)
	}
	for _, path := range []string{"", "A", "A/A1", "A/A2"} {
		if _, ok := s.States[path]; !ok {
			t.Fatalf("missing state %q", path)
		}
	}
	if s.States["A/A1"].Parent != "A" || s.States["A/A1"].Name != "A1" || !s.IsLeaf("A/A2") || s.IsLeaf("A") {
		t.Fatalf("hierarchy: %+v", s.States)
	}
	for _, id := range []string{":[開始]", ":終了", "A:[開始]", "A/A1:次", "A/A1:判定", "A/A1:判定>可", "A/A1:判定>否"} {
		if _, ok := s.Transitions[id]; !ok {
			t.Fatalf("missing transition %q in %v", id, model.Keys(s.Transitions))
		}
	}
	d := s.Decisions["A/A1:判定?"]
	if d.Parent != "A" || d.Owner != "A/A1:判定" || len(d.Branches) != 2 || d.Label != "結果" {
		t.Fatalf("decision: %+v", d)
	}
	branch := s.Transitions["A/A1:判定>否"]
	if branch.Kind != model.KindBranch || branch.From != "A/A1:判定?" || branch.To != "" || branch.Root != "A/A1:判定" {
		t.Fatalf("staying branch: %+v", branch)
	}
	if event := s.Transitions["A/A1:判定"]; event.To != "A/A1:判定?" || event.Action != "確認する" {
		t.Fatalf("event entering decision: %+v", event)
	}
	if u := s.UseCases["u"]; u.Starts[0] != "A/A1" || u.Ends[0] != "A/A1:次" || u.Scope[0] != "A/A1:次" {
		t.Fatalf("use-case references not resolved: %+v", u)
	}
	if leaf, err := s.Enter(""); err != nil || leaf != "A/A1" {
		t.Fatalf("root start enters %q, %v", leaf, err)
	}
	if src := s.Sources["transitions/A/A1:判定>可"]; src.File == "" || !strings.Contains(src.Pointer, "/decision/branches/") {
		t.Fatalf("branch source: %+v", src)
	}
}

func TestReferenceResolution(t *testing.T) {
	s := testkit.Compile(t, `{"schemaVersion":"1","start":{"to":"X"},"actors":{"人":{}},"states":{
	 "X":{"start":{"to":"S"},"states":{"S":{"events":{"e":{"actors":["人"],"decision":{"branches":{"a":{},"b":{}}}}}}}},
	 "Y":{"start":{"to":"S"},"states":{"S":{}}}}}`)
	if _, err := compiler.ResolveState(s, "S"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous suffix accepted: %v", err)
	}
	if path, err := compiler.ResolveState(s, "X/S"); err != nil || path != "X/S" {
		t.Fatal(path, err)
	}
	if _, err := compiler.ResolveState(s, "Z"); err == nil {
		t.Fatal("unknown state accepted")
	}
	if id, err := compiler.ResolveTransition(s, "X/S:e>a"); err != nil || id != "X/S:e>a" {
		t.Fatal(id, err)
	}
	if id, err := compiler.ResolveTransition(s, "X:[開始]"); err != nil || id != "X:[開始]" {
		t.Fatal(id, err)
	}
	for _, ref := range []string{"X/S", "X/S:none", "X/S:e>c", "Y/S:e"} {
		if _, err := compiler.ResolveTransition(s, ref); err == nil {
			t.Fatalf("%s accepted", ref)
		}
	}
}

func TestStructuralProblems(t *testing.T) {
	usecase := `"usecases":{"u":{"name":"U","goal":"G","actor":"人","starts":["A"],"ends":["A:e"],"scope":["A:e"],"scenarios":{}}}`
	for name, tc := range map[string]struct{ doc, want string }{
		"leaf start":        {`{"schemaVersion":"1","start":{"to":"A"},"actors":{"人":{}},"states":{"A":{"start":{"to":"A"},"events":{"e":{"actors":["人"]}}}},` + usecase + `}`, "leaf state must not have a start"},
		"composite start":   {`{"schemaVersion":"1","start":{"to":"P"},"actors":{"人":{}},"states":{"P":{"states":{"A":{"events":{"e":{"actors":["人"]}}}}}},` + usecase + `}`, "requires a start"},
		"start stays":       {`{"schemaVersion":"1","start":{"to":"P"},"actors":{"人":{}},"states":{"P":{"start":{"action":"x"},"states":{"A":{"events":{"e":{"actors":["人"]}}}}}},` + usecase + `}`, "start must have a destination"},
		"start outside":     {`{"schemaVersion":"1","start":{"to":"P"},"actors":{"人":{}},"states":{"P":{"start":{"to":"B"},"states":{"A":{"events":{"e":{"actors":["人"]}}}}},"B":{}},` + usecase + `}`, "must enter a state inside"},
		"to and decision":   {`{"schemaVersion":"1","start":{"to":"A"},"actors":{"人":{}},"states":{"A":{"events":{"e":{"actors":["人"],"to":"A","decision":{"branches":{"a":{},"b":{}}}}}}},` + usecase + `}`, "exclusive"},
		"single branch":     {`{"schemaVersion":"1","start":{"to":"A"},"actors":{"人":{}},"states":{"A":{"events":{"e":{"actors":["人"],"decision":{"branches":{"a":{}}}}}}},` + usecase + `}`, "at least two branches"},
		"bad name":          {`{"schemaVersion":"1","start":{"to":"A"},"actors":{"人":{}},"states":{"A":{"events":{"e":{"actors":["人"]},"x>y":{"actors":["人"]}}}},` + usecase + `}`, "name must be"},
		"unknown actor":     {`{"schemaVersion":"1","start":{"to":"A"},"actors":{"人":{}},"states":{"A":{"events":{"e":{"actors":["他"]}}}},` + usecase + `}`, "unknown actor 他"},
		"unknown target":    {`{"schemaVersion":"1","start":{"to":"A"},"actors":{"人":{}},"states":{"A":{"events":{"e":{"actors":["人"],"to":"Z"}}}},` + usecase + `}`, "unknown state"},
		"missing root":      {`{"schemaVersion":"1","actors":{"人":{}},"states":{"A":{"events":{"e":{"actors":["人"]}}}},` + usecase + `}`, "root requires a start"},
		"actor scope":       {`{"schemaVersion":"1","start":{"to":"A"},"actors":{"人":{},"他":{}},"states":{"A":{"events":{"e":{"actors":["他"]}}}},` + usecase + `}`, "cannot be raised by"},
		"end outside scope": {`{"schemaVersion":"1","start":{"to":"A"},"actors":{"人":{}},"states":{"A":{"events":{"e":{"actors":["人"]},"f":{"actors":["人"]}}}},"usecases":{"u":{"name":"U","goal":"G","actor":"人","starts":["A"],"ends":["A:f"],"scope":["A:e"],"scenarios":{}}}}`, "outside the scope"},
		"regression actor":  {`{"schemaVersion":"1","start":{"to":"A"},"actors":{"人":{}},"states":{"A":{"events":{"e":{"actors":["人"]}}}},` + usecase + `,"regressions":{"r":{"name":"R","goal":"G","expectedOutcome":"O","start":"A","steps":[{"event":"e"}]}}}`, "require a known actor"},
	} {
		t.Run(name, func(t *testing.T) {
			s := testkit.Compile(t, tc.doc)
			found := []string{}
			for _, d := range compiler.Validate(s) {
				found = append(found, d.Subject+": "+d.Message)
				if strings.Contains(d.Message, tc.want) {
					return
				}
			}
			t.Fatalf("want %q, got %v", tc.want, found)
		})
	}
}

func TestNamesThatLookTheSameAreRejected(t *testing.T) {
	s, err := compiler.FromDocuments(testkit.Project(), map[string]model.Document{"a.json": {SchemaVersion: "1",
		Start:  &model.Target{To: "A"},
		Actors: map[string]model.Actor{"人": {}},
		States: map[string]model.State{"A": {}, "Ａ": {}}}}, func(name string) string { return strings.ReplaceAll(name, "Ａ", "A") })
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range compiler.Validate(s) {
		if strings.Contains(d.Message, "looks the same") {
			return
		}
	}
	t.Fatal("normalization collision accepted")
}

func TestRootStartAppearsOnceAcrossFiles(t *testing.T) {
	_, err := testkit.CompileError(`{"schemaVersion":"1","start":{"to":"A"},"states":{"A":{}}}`, `{"schemaVersion":"1","start":{"to":"A"}}`)
	if err == nil || !strings.Contains(err.Error(), "duplicate root start") {
		t.Fatalf("duplicate root start: %v", err)
	}
	_, err = testkit.CompileError(`{"schemaVersion":"1","states":{"A":{}}}`, `{"schemaVersion":"1","states":{"A":{}}}`)
	if err == nil || !strings.Contains(err.Error(), "duplicate states/A") {
		t.Fatalf("duplicate top-level state: %v", err)
	}
}

func TestSampleCompilesWithoutProblems(t *testing.T) {
	s := testkit.Sample(t)
	if d := compiler.Validate(s); len(d) != 0 {
		t.Fatal(d)
	}
	if got := s.Transitions["顧客管理/顧客編集:設定を選択"]; got.Kind != model.KindEvent || got.To != "顧客管理/顧客編集:設定を選択?" {
		t.Fatalf("overriding menu event: %+v", got)
	}
}
