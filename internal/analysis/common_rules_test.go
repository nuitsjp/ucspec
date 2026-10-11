package analysis

import (
	"testing"

	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/testkit/choicefixture"
)

func TestCommonRulesAffectReviewWithoutChangingFiniteResults(t *testing.T) {
	s := choicefixture.ChoiceSpec()
	before := Analyze(s, "test")
	s.CommonRules["systemError"] = model.CommonRule{Name: "System error", Description: "Keep committed data."}
	after := Analyze(s, "test")
	if after.Definitions["commonRules/systemError"] == "" {
		t.Fatal("common rule definition missing")
	}
	if before.Status != after.Status || before.ReachableStates != after.ReachableStates || model.JSON(before.Checks) != model.JSON(after.Checks) || model.JSON(before.Scenarios) != model.JSON(after.Scenarios) || model.JSON(useCasesWithoutReviewBasis(before.UseCases)) != model.JSON(useCasesWithoutReviewBasis(after.UseCases)) {
		t.Fatal("narrative common rule changed finite results")
	}
	if before.UseCases[0].ReviewBasisHash == after.UseCases[0].ReviewBasisHash {
		t.Fatal("common rule did not invalidate the review basis")
	}
	if err := Compare(after, before); err != nil {
		t.Fatal(err)
	}
	if model.JSON(after.Impact.AffectedUseCases) != model.JSON([]string{"search"}) {
		t.Fatalf("common rule addition did not affect review: %+v", after.Impact)
	}
	s.CommonRules["systemError"] = model.CommonRule{Name: "System error", Description: "Keep committed data and draft."}
	changed := Analyze(s, "test")
	if err := Compare(changed, after); err != nil {
		t.Fatal(err)
	}
	if model.JSON(changed.Impact.Changed) != model.JSON([]string{"commonRules/systemError"}) || model.JSON(changed.Impact.AffectedUseCases) != model.JSON([]string{"search"}) {
		t.Fatalf("common rule edit not tracked: %+v", changed.Impact)
	}
}
