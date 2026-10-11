package compiler

import (
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/model"
)

func hasTransitionDiagnostic(s *model.Spec, message string) bool {
	for _, d := range Validate(s) {
		if d.Subject == "transitions/internal" && d.Severity == "error" && strings.Contains(d.Message, message) {
			return true
		}
	}
	return false
}

func TestInternalTransitionValidation(t *testing.T) {
	cases := []struct {
		name    string
		change  func(*model.Spec)
		message string
	}{
		{
			name: "unknown kind",
			change: func(s *model.Spec) {
				s.Transitions["internal"] = model.Transition{From: "customer", To: "customer", Kind: "loop", Event: "close", Action: "Loop"}
			},
			message: "kind must be empty, external or internal",
		},
		{
			name: "different target",
			change: func(s *model.Spec) {
				s.Transitions["internal"] = model.Transition{From: "customer", To: "done", Kind: "internal", Event: "close", Action: "Loop"}
			},
			message: "same from and to state",
		},
		{
			name: "choice source",
			change: func(s *model.Spec) {
				s.Choices["branch"] = model.Choice{Label: "Branch"}
				s.Transitions["internal"] = model.Transition{From: "branch", To: "viewing", Kind: "internal", Guard: true, Action: "Loop"}
			},
			message: "internal transitions must use state endpoints",
		},
		{
			name: "choice target",
			change: func(s *model.Spec) {
				s.Choices["branch"] = model.Choice{Label: "Branch"}
				s.Transitions["internal"] = model.Transition{From: "viewing", To: "branch", Kind: "internal", Event: "ask", Action: "Loop"}
			},
			message: "internal transitions must use state endpoints",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := hierarchicalSpec()
			tc.change(s)
			if !hasTransitionDiagnostic(s, tc.message) {
				t.Fatalf("missing diagnostic %q: %v", tc.message, Validate(s))
			}
		})
	}
}
