package analysis

import (
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/model"
)

func TestCoverageReviewsDoNotChangeCoverageOrVerdicts(t *testing.T) {
	for _, status := range []string{"addScenario", "represented", "forbidden"} {
		for _, threshold := range []bool{false, true} {
			name := status
			if threshold {
				name += "/failing-threshold"
			}
			t.Run(name, func(t *testing.T) {
				s := convergingStartsSpec()
				u := s.UseCases["flow"]
				delete(u.Scenarios, "fromB")
				s.UseCases["flow"] = u
				if threshold {
					s.Project.Coverage = map[string]float64{"transitions": 100}
				}
				before := Analyze(s, "test")
				review := model.CoverageReview{
					Kind: "transitions", Requirements: []string{"tb", "finish"}, Status: status,
					Reason: "Review the second start separately", BasisHash: before.UseCases[0].ReviewBasisHash,
					ArchitectureRequirements: []string{"Keep confirmation input until completion"},
				}
				if status == "represented" {
					review.Scenario = "fromA"
				}
				u.CoverageReviews = map[string]model.CoverageReview{"secondStart": review}
				s.UseCases["flow"] = u
				after := Analyze(s, "test")
				if !model.Equal(before.UseCases[0].Coverage, after.UseCases[0].Coverage) || before.Status != after.Status || before.Complete != after.Complete || !model.Equal(before.Checks, after.Checks) || !model.Equal(before.Diagnostics, after.Diagnostics) {
					t.Fatal("review changed coverage or verification results")
				}
				if after.UseCases[0].ReviewBasisHash != review.BasisHash {
					t.Fatal("adding a review invalidated its own context hash")
				}
				reviews := after.UseCases[0].CoverageReviews
				if len(reviews) != 1 || reviews[0].Status != "current" || reviews[0].Error != "" || !model.Equal(reviews[0].Review, review) {
					t.Fatalf("unexpected review result: %+v", reviews)
				}
				for _, scenario := range after.Scenarios {
					if scenario.Goal == "" || scenario.ExpectedOutcome == "" {
						t.Fatal("scenario purpose or expected outcome was lost")
					}
				}
			})
		}
	}
}

func TestCoverageReviewsBecomeStale(t *testing.T) {
	for _, reason := range []string{"context", "missing", "scenario", "incomplete"} {
		t.Run(reason, func(t *testing.T) {
			s := convergingStartsSpec()
			u := s.UseCases["flow"]
			delete(u.Scenarios, "fromB")
			if reason == "incomplete" {
				s.Project.Limits.States = 1
			}
			if reason == "scenario" {
				scenario := u.Scenarios["fromA"]
				scenario.Expect = false
				u.Scenarios["fromA"] = scenario
			}
			s.UseCases["flow"] = u
			before := Analyze(s, "test")
			review := model.CoverageReview{Kind: "transitions", Requirements: []string{"tb"}, Status: "represented", Scenario: "fromA", Reason: "Same purpose", BasisHash: before.UseCases[0].ReviewBasisHash}
			wantError := ""
			switch reason {
			case "context":
				review.BasisHash = "outdated"
				wantError = "verification context has changed"
			case "missing":
				review.Requirements = []string{"ta"}
				wantError = "is no longer missing"
			case "scenario":
				wantError = "representative scenario is invalid"
			case "incomplete":
				wantError = "coverage analysis is incomplete"
			}
			u.CoverageReviews = map[string]model.CoverageReview{"secondStart": review}
			s.UseCases["flow"] = u
			after := Analyze(s, "test")
			if before.Status != after.Status || !model.Equal(before.UseCases[0].Coverage, after.UseCases[0].Coverage) {
				t.Fatal("stale review changed the verdict or coverage")
			}
			results := after.UseCases[0].CoverageReviews
			if len(results) != 1 || results[0].Status != "stale" || !strings.Contains(results[0].Error, wantError) {
				t.Fatalf("expected stale review (%s): %+v", wantError, results)
			}
		})
	}
}

func TestCoverageReviewsSupportAllCoverageKinds(t *testing.T) {
	s := hierarchicalSpec()
	u := s.UseCases["delete"]
	u.Scenarios = map[string]model.Scenario{}
	s.UseCases["delete"] = u
	s.Project.Coverage = map[string]float64{"states": 100, "compositeStates": 100}
	before := Analyze(s, "test")
	if before.Status != "failed" || !before.Complete || len(before.UseCases) != 1 {
		t.Fatalf("unexpected hierarchy baseline: %+v", before)
	}
	u.CoverageReviews = map[string]model.CoverageReview{}
	for _, coverage := range before.UseCases[0].Coverage {
		if len(coverage.Missing) == 0 {
			t.Fatalf("fixture has no missing %s requirement", coverage.Kind)
		}
		requirements := []string{}
		for _, requirement := range coverage.Missing {
			requirements = append(requirements, requirement.ID)
		}
		u.CoverageReviews[coverage.Kind] = model.CoverageReview{
			Kind: coverage.Kind, Requirements: requirements, Status: "addScenario",
			Reason:    "Add scenarios that exercise the unvisited states and paths",
			BasisHash: before.UseCases[0].ReviewBasisHash,
		}
	}
	s.UseCases["delete"] = u
	after := Analyze(s, "test")
	if after.Status != before.Status || after.Complete != before.Complete || !model.Equal(after.Checks, before.Checks) || !model.Equal(after.Diagnostics, before.Diagnostics) || !model.Equal(after.UseCases[0].Coverage, before.UseCases[0].Coverage) {
		t.Fatal("review changed hierarchy coverage or a failing state threshold")
	}
	if len(after.UseCases[0].CoverageReviews) != len(model.CoverageKinds) {
		t.Fatalf("not all eight coverage kinds were reviewed: %+v", after.UseCases[0].CoverageReviews)
	}
	for _, review := range after.UseCases[0].CoverageReviews {
		if review.Status != "current" || review.Error != "" {
			t.Fatalf("hierarchy review rejected: %+v", review)
		}
	}
}

func TestCoverageReviewBasisTracksVerificationContext(t *testing.T) {
	for _, change := range []string{"model", "scenario", "limits", "property", "version"} {
		t.Run(change, func(t *testing.T) {
			s := convergingStartsSpec()
			before := reviewBasisHash(s, "flow", "test")
			version := "test"
			switch change {
			case "model":
				tr := s.Transitions["ta"]
				tr.Action = "changed action"
				s.Transitions["ta"] = tr
			case "scenario":
				u := s.UseCases["flow"]
				scenario := u.Scenarios["fromA"]
				scenario.ExpectedOutcome = "revised expected outcome"
				u.Scenarios["fromA"] = scenario
			case "limits":
				s.Project.Limits.Steps++
			case "property":
				s.Properties["safe"] = model.Property{Kind: "invariant", Description: "Safety", Assert: true}
			case "version":
				version = "next"
			}
			if reviewBasisHash(s, "flow", version) == before {
				t.Fatalf("%s change did not invalidate the review basis", change)
			}
		})
	}
	s := convergingStartsSpec()
	before := reviewBasisHash(s, "flow", "test")
	u := s.UseCases["flow"]
	u.CoverageReviews = map[string]model.CoverageReview{"note": {Reason: "editorial change"}}
	s.UseCases["flow"] = u
	s.UseCases["another"] = u
	if reviewBasisHash(s, "flow", "test") != before {
		t.Fatal("reviews or another use case changed this use case's review basis")
	}
}
