package model

import "testing"

func TestStableHash(t *testing.T) {
	a := map[string]any{"a": 1, "b": 2}
	b := map[string]any{"b": 2, "a": 1}
	if Hash(a) != Hash(b) {
		t.Fatal("map order changes hash")
	}
}

func TestStatePathsAndEntering(t *testing.T) {
	s := NewSpec()
	s.States[""] = StateNode{Start: StartID(""), Children: []string{"P"}}
	s.States["P"] = StateNode{Name: "P", Start: StartID("P"), Children: []string{"P/A", "P/B"}}
	s.States["P/A"] = StateNode{Name: "A", Parent: "P"}
	s.States["P/B"] = StateNode{Name: "B", Parent: "P"}
	s.Transitions[StartID("")] = Transition{Kind: KindStart, To: "P"}
	s.Transitions[StartID("P")] = Transition{Kind: KindStart, From: "P", To: "P/B"}
	if got := s.StatePath("P/A"); len(got) != 3 || got[0] != "P/A" || got[1] != "P" || got[2] != "" {
		t.Fatalf("state path: %v", got)
	}
	if !s.InState("P/A", "P") || !s.InState("P/A", "") || s.InState("P/A", "P/B") {
		t.Fatal("in-state check")
	}
	if leaf, err := s.Enter(""); err != nil || leaf != "P/B" {
		t.Fatalf("enter root: %q %v", leaf, err)
	}
	if got := s.Leaves("P"); len(got) != 2 {
		t.Fatalf("leaves: %v", got)
	}
	if !s.IsLeaf("P/A") || s.IsLeaf("P") || s.IsLeaf("") {
		t.Fatal("leaf check")
	}
}

func TestValidNames(t *testing.T) {
	for _, name := range []string{"顧客検索", "1件以上", "破棄する"} {
		if !ValidName(name) {
			t.Errorf("%q rejected", name)
		}
	}
	for _, name := range []string{"", " 前", "後 ", "a/b", "a:b", "a>b", "a?b", "[開始]"} {
		if ValidName(name) {
			t.Errorf("%q accepted", name)
		}
	}
}
