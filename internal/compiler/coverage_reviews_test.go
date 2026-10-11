package compiler

import (
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/model"
)

func scenarioMetadataSpec() *model.Spec {
	s := &model.Spec{Document: model.EmptyDocument(), Project: model.Project{
		SchemaVersion: "1", Name: "scenario metadata", Files: []string{"model.json"}, Output: "out",
		Initial: []model.Snapshot{{State: "start"}}, Limits: model.Limits{States: 10, Steps: 2, Paths: 100, Combinations: 10},
	}}
	s.Screens["screen"] = model.Screen{Label: "screen"}
	s.States["start"] = model.State{Label: "start", Screen: "screen", Events: []string{"go"}}
	s.States["end"] = model.State{Label: "end", Screen: "screen", Terminal: true}
	s.Events["go"] = model.Event{Label: "go", Source: "user"}
	s.Transitions["finish"] = model.Transition{From: "start", To: "end", Event: "go", Action: "finish"}
	end := map[string]any{"eq": []any{map[string]any{"var": "$state"}, "end"}}
	scenario := model.Scenario{Name: "finish", Goal: "complete the task", ExpectedOutcome: "the task is complete", Start: model.Snapshot{State: "start"}, Steps: []model.Step{{Transition: "finish"}}, Expect: end}
	s.UseCases["flow"] = model.UseCase{Name: "flow", Goal: "complete", Actor: "user", Starts: s.Project.Initial, Scope: []string{"finish"}, End: end, Scenarios: map[string]model.Scenario{"normal": scenario}}
	s.Regressions["normal"] = scenario
	return s
}

func TestScenarioGoalAndExpectedOutcomeAreRequired(t *testing.T) {
	if ds := Validate(scenarioMetadataSpec()); len(ds) != 0 {
		t.Fatalf("invalid baseline: %+v", ds)
	}
	for _, section := range []string{"usecases", "regressions"} {
		for _, field := range []string{"goal", "expectedOutcome"} {
			for _, value := range []string{"", " \t\n\u3000"} {
				t.Run(section+"/"+field+"/"+value, func(t *testing.T) {
					s := scenarioMetadataSpec()
					u := s.UseCases["flow"]
					scenario := u.Scenarios["normal"]
					if field == "goal" {
						scenario.Goal = value
					} else {
						scenario.ExpectedOutcome = value
					}
					if section == "usecases" {
						u.Scenarios["normal"] = scenario
					} else {
						s.Regressions["normal"] = scenario
					}
					for _, d := range Validate(s) {
						if d.Code == "structure" && strings.HasPrefix(d.Subject, section+"/") && d.Message == "scenario "+field+" required" {
							return
						}
					}
					t.Fatalf("missing %s validation for %s", field, section)
				})
			}
		}
	}
}

func TestCoverageReviewStructureAndReferences(t *testing.T) {
	for _, change := range []string{"valid", "id", "kind", "status", "reason", "basisHash", "requirements", "blankRequirement", "duplicateRequirement", "duplicateReview", "missingScenario", "unknownScenario", "architectureRequirement"} {
		t.Run(change, func(t *testing.T) {
			s := scenarioMetadataSpec()
			u := s.UseCases["flow"]
			id := "review"
			review := model.CoverageReview{Kind: "transitions", Requirements: []string{"finish"}, Status: "represented", Reason: "Same purpose", Scenario: "normal", BasisHash: "recorded-context"}
			switch change {
			case "id":
				id = "bad id"
			case "kind":
				review.Kind = "unknown"
			case "status":
				review.Status = "approved"
			case "reason":
				review.Reason = " \t\u3000"
			case "basisHash":
				review.BasisHash = " "
			case "requirements":
				review.Requirements = nil
			case "blankRequirement":
				review.Requirements = []string{" "}
			case "duplicateRequirement":
				review.Requirements = []string{"finish", "finish"}
			case "missingScenario":
				review.Scenario = ""
			case "unknownScenario":
				review.Scenario = "missing"
			case "architectureRequirement":
				review.ArchitectureRequirements = []string{" "}
			}
			u.CoverageReviews = map[string]model.CoverageReview{id: review}
			if change == "duplicateReview" {
				u.CoverageReviews["other"] = review
			}
			s.UseCases["flow"] = u
			diagnostics := Validate(s)
			if change == "valid" {
				if len(diagnostics) != 0 {
					t.Fatalf("valid review rejected: %+v", diagnostics)
				}
				return
			}
			for _, d := range diagnostics {
				if d.Code == "structure" && d.Subject == "usecases/flow" && strings.Contains(d.Message, "coverageReviews/") {
					return
				}
			}
			t.Fatalf("invalid review accepted: %s", change)
		})
	}
}
