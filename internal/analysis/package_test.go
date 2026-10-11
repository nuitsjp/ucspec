package analysis

import (
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/testkit"
)

func nestedPackageImpactSample(t *testing.T) *model.Spec {
	t.Helper()
	s := testkit.Sample(t)
	for id := range s.Sources {
		if strings.HasPrefix(id, "usecasePackages/") {
			delete(s.Sources, id)
		}
	}
	s.UseCasePackages = map[string]model.UseCasePackage{
		"customerManagement": {Name: "顧客管理"},
		"customerRecords":    {Name: "顧客情報", Parent: "customerManagement"},
		"operations":         {Name: "運用設定"},
		"notifications":      {Name: "通知設定", Parent: "operations"},
		"unused":             {Name: "未使用"},
	}
	for _, id := range []string{"deleteCustomer", "editCustomer", "searchCustomer", "viewCustomer"} {
		u := s.UseCases[id]
		u.Package = "customerRecords"
		s.UseCases[id] = u
	}
	u := s.UseCases["updateNotifications"]
	u.Package = "notifications"
	s.UseCases["updateNotifications"] = u
	return s
}

func TestUseCasePackagesDoNotChangeExecutionOrCoverage(t *testing.T) {
	s := testkit.Sample(t)
	packaged := Analyze(s, "test")
	for id, u := range s.UseCases {
		u.Package = ""
		s.UseCases[id] = u
	}
	for id := range s.UseCasePackages {
		delete(s.Sources, "usecasePackages/"+id)
	}
	s.UseCasePackages = map[string]model.UseCasePackage{}
	root := Analyze(s, "test")
	if packaged.Status != "verified" || root.Status != "verified" || !root.Complete {
		t.Fatalf("package metadata affected verification: %s, %s", packaged.Status, root.Status)
	}
	if packaged.ReachableStates != root.ReachableStates || model.JSON(packaged.Checks) != model.JSON(root.Checks) || model.JSON(packaged.Scenarios) != model.JSON(root.Scenarios) || model.JSON(useCasesWithoutReviewBasis(packaged.UseCases)) != model.JSON(useCasesWithoutReviewBasis(root.UseCases)) {
		t.Fatal("package metadata changed execution, checks, scenarios or coverage")
	}
	for i, u := range packaged.UseCases {
		if u.ReviewBasisHash == root.UseCases[i].ReviewBasisHash {
			t.Fatalf("package changes did not invalidate the review basis for %s", u.ID)
		}
	}
}

func TestUseCasePackageImpactIncludesAncestors(t *testing.T) {
	customerUseCases := []string{"deleteCustomer", "editCustomer", "searchCustomer", "viewCustomer"}
	cases := []struct {
		name     string
		change   func(*model.Spec)
		affected []string
	}{
		{
			name: "rename ancestor",
			change: func(s *model.Spec) {
				p := s.UseCasePackages["customerManagement"]
				p.Name = "顧客業務"
				s.UseCasePackages["customerManagement"] = p
			},
			affected: customerUseCases,
		},
		{
			name: "move package to another branch",
			change: func(s *model.Spec) {
				p := s.UseCasePackages["customerRecords"]
				p.Parent = "operations"
				s.UseCasePackages["customerRecords"] = p
			},
			affected: customerUseCases,
		},
		{
			name: "move one use case",
			change: func(s *model.Spec) {
				u := s.UseCases["viewCustomer"]
				u.Package = "notifications"
				s.UseCases["viewCustomer"] = u
			},
			affected: []string{"viewCustomer"},
		},
		{
			name: "rename empty package",
			change: func(s *model.Spec) {
				s.UseCasePackages["unused"] = model.UseCasePackage{Name: "未使用の分類"}
			},
			affected: []string{},
		},
		{
			name: "remove former ancestors after moving use cases",
			change: func(s *model.Spec) {
				for _, id := range customerUseCases {
					u := s.UseCases[id]
					u.Package = "notifications"
					s.UseCases[id] = u
				}
				delete(s.UseCasePackages, "customerRecords")
				delete(s.UseCasePackages, "customerManagement")
			},
			affected: customerUseCases,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := nestedPackageImpactSample(t)
			s.UseCasePackages["unused"] = model.UseCasePackage{Name: "未使用"}
			old := Analyze(s, "test")
			tc.change(s)
			now := Analyze(s, "test")
			if now.Status != "verified" {
				t.Fatalf("package change invalidated the model: %+v", now.Diagnostics)
			}
			if err := Compare(now, old); err != nil {
				t.Fatal(err)
			}
			if model.JSON(now.Impact.AffectedUseCases) != model.JSON(tc.affected) {
				t.Fatalf("affected use cases: got %v, want %v", now.Impact.AffectedUseCases, tc.affected)
			}
			if len(now.Impact.InvalidatedScenarios) != 0 || len(now.Impact.CoverageChanges) != 0 {
				t.Fatalf("package change affected scenario validity or coverage: %+v", now.Impact)
			}
		})
	}
}

func TestAnalyzeRejectsPackageCycle(t *testing.T) {
	s := nestedPackageImpactSample(t)
	p := s.UseCasePackages["customerManagement"]
	p.Parent = "customerRecords"
	s.UseCasePackages["customerManagement"] = p
	r := Analyze(s, "test")
	if r.Status != "failed" || r.Complete {
		t.Fatalf("cyclic package hierarchy accepted: %s, complete=%v", r.Status, r.Complete)
	}
	for _, d := range r.Diagnostics {
		if d.Subject == "usecasePackages/customerManagement" && d.Severity == "error" {
			return
		}
	}
	t.Fatal("package cycle diagnostic absent")
}
