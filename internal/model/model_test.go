package model

import "testing"

func TestNormalizeDomainsAndDefaults(t *testing.T) {
	s := &Spec{Document: EmptyDocument()}
	s.States["a"] = State{}
	s.Variables["x"] = Variable{Domain: []any{0, 1}, Initial: 0}
	n, e := s.Normalize(Snapshot{State: "a"})
	if e != nil || !Equal(n.Values["x"], 0) {
		t.Fatal(n, e)
	}
	for _, bad := range []Snapshot{{State: "missing"}, {State: "a", Values: map[string]any{"x": 2}}, {State: "a", Values: map[string]any{"unknown": 1}}} {
		if _, e = s.Normalize(bad); e == nil {
			t.Fatal("invalid snapshot accepted")
		}
	}
}
func TestStableHashAndCloning(t *testing.T) {
	a := map[string]any{"a": 1, "b": 2}
	b := map[string]any{"b": 2, "a": 1}
	if Hash(a) != Hash(b) {
		t.Fatal("map order changes hash")
	}
	s := Snapshot{State: "s", Values: a}
	x := CloneSnapshot(s)
	x.Values["a"] = 9
	if Equal(s.Values["a"], 9) {
		t.Fatal("mutable alias")
	}
}
