package analysis

import (
	"github.com/nuitsjp/usecase-spec/internal/testkit"
	"testing"
)

func TestSample(t *testing.T) {
	s := testkit.Sample(t)
	r := Analyze(s, "test")
	for _, d := range r.Diagnostics {
		t.Log(d.Code, d.Subject, d.Message)
	}
	for _, u := range r.UseCases {
		for _, c := range u.Coverage {
			t.Log(u.ID, c.Kind, c.Covered, c.Total, c.Complete, "infeasible", len(c.Infeasible))
		}
	}
	for _, c := range r.Checks {
		if c.Status != "verified" {
			t.Errorf("check %s: %+v", c.ID, c)
		}
	}
	if r.Status != "verified" {
		t.Fatalf("sample status %s", r.Status)
	}
}
