package analysis

import (
	"fmt"
	"strings"

	"github.com/nuitsjp/ucspec/internal/model"
)

func reviewBasisHash(s *model.Spec, useCaseID, version string) string {
	document := s.Document
	document.UseCases = nil
	u := s.UseCases[useCaseID]
	u.CoverageReviews = nil
	return model.Hash(map[string]any{
		"toolVersion": version,
		"project":     s.Project,
		"model":       document,
		"usecaseID":   useCaseID,
		"usecase":     u,
	})
}

// Reviews annotate missing requirements; they never change coverage or verdicts.
func evaluateCoverageReviews(u model.UseCase, result model.UseCaseResult, scenarios []model.ScenarioResult) []model.CoverageReviewResult {
	missing := map[[2]string]bool{}
	for _, coverage := range result.Coverage {
		for _, requirement := range coverage.Missing {
			missing[[2]string{coverage.Kind, requirement.ID}] = true
		}
	}
	validScenarios := map[string]bool{}
	for _, scenario := range scenarios {
		validScenarios[scenario.ID] = scenario.Valid
	}
	reviews := []model.CoverageReviewResult{}
	for _, id := range model.Keys(u.CoverageReviews) {
		review := u.CoverageReviews[id]
		r := model.CoverageReviewResult{ID: id, Review: review, Status: "current"}
		errors := []string{}
		if !result.Complete {
			errors = append(errors, "coverage analysis is incomplete")
		}
		if review.BasisHash != result.ReviewBasisHash {
			errors = append(errors, "verification context has changed")
		}
		for _, requirement := range review.Requirements {
			if !missing[[2]string{review.Kind, requirement}] {
				errors = append(errors, fmt.Sprintf("%s requirement %q is no longer missing", review.Kind, requirement))
			}
		}
		if review.Status == "represented" && !validScenarios[result.ID+"/"+review.Scenario] {
			errors = append(errors, "representative scenario is invalid")
		}
		if len(errors) > 0 {
			r.Status = "stale"
			r.Error = strings.Join(errors, "; ")
		}
		reviews = append(reviews, r)
	}
	return reviews
}
