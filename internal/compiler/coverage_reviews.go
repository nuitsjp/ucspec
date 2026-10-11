package compiler

import (
	"fmt"
	"strings"

	"github.com/nuitsjp/ucspec/internal/model"
)

func validateCoverageReviews(u model.UseCase) []string {
	messages := []string{}
	seen := map[[2]string]string{}
	for _, id := range model.Keys(u.CoverageReviews) {
		r := u.CoverageReviews[id]
		add := func(message string) { messages = append(messages, "coverageReviews/"+id+": "+message) }
		if !idRE.MatchString(id) {
			add("invalid review ID")
		}
		if !model.Contains(model.CoverageKinds, r.Kind) {
			add("unknown coverage kind " + r.Kind)
		}
		if !model.Contains([]string{"addScenario", "represented", "forbidden"}, r.Status) {
			add("unknown review status " + r.Status)
		}
		if strings.TrimSpace(r.Reason) == "" {
			add("reason required")
		}
		if strings.TrimSpace(r.BasisHash) == "" {
			add("basisHash required")
		}
		if len(r.Requirements) == 0 {
			add("at least one requirement ID required")
		}
		for _, requirement := range r.Requirements {
			if strings.TrimSpace(requirement) == "" {
				add("requirement ID must not be blank")
			}
			key := [2]string{r.Kind, requirement}
			if previous, ok := seen[key]; ok {
				add(fmt.Sprintf("duplicate %s requirement %q (already reviewed by %s)", r.Kind, requirement, previous))
			}
			seen[key] = id
		}
		if r.Status == "represented" && r.Scenario == "" {
			add("represented review requires a scenario")
		}
		if r.Scenario != "" {
			if _, ok := u.Scenarios[r.Scenario]; !ok {
				add("unknown scenario " + r.Scenario)
			}
		}
		for _, requirement := range r.ArchitectureRequirements {
			if strings.TrimSpace(requirement) == "" {
				add("architecture requirement must not be blank")
			}
		}
	}
	return messages
}
