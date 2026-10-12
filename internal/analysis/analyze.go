package analysis

import (
	"fmt"
	"sort"

	"github.com/nuitsjp/ucspec/internal/compiler"
	"github.com/nuitsjp/ucspec/internal/engine"
	"github.com/nuitsjp/ucspec/internal/model"
)

func Analyze(s *model.Spec, version string) *model.Report {
	r := &model.Report{FormatVersion: "1", ToolVersion: version, SchemaVersion: s.Project.SchemaVersion, Name: s.Project.Name, InputHash: s.Hash, Limits: s.Project.Limits, Status: "verified", Complete: true, Diagnostics: []model.Diagnostic{}, Checks: []model.Check{}, Scenarios: []model.ScenarioResult{}, UseCases: []model.UseCaseResult{}, Definitions: map[string]string{}, Dependencies: map[string][]string{}}
	r.Diagnostics = compiler.Validate(s)
	definitions(s, r)
	if hasErrors(r.Diagnostics) {
		r.Status = "failed"
		r.Complete = false
		finish(r)
		return r
	}
	e := engine.New(s)
	global := Explore(e, []string{""}, "", nil, nil)
	r.Complete = global.Complete
	r.Diagnostics = append(r.Diagnostics, global.Diagnostics...)
	reached := map[string]bool{}
	reachedTransitions := map[string]bool{}
	for _, n := range global.Nodes {
		if s.IsLeaf(n.State.State) {
			r.ReachableStates++
		}
		for _, id := range s.StatePath(n.State.State) {
			reached[id] = true
		}
	}
	for _, ed := range global.Edges {
		reachedTransitions[ed.Transition] = true
	}
	if global.Complete {
		for _, id := range model.Keys(s.States) {
			if id != "" && !reached[id] {
				r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "unreachable-state", Severity: "warning", Subject: "states/" + id, Message: "state is unreachable from the root start", Source: s.Sources["states/"+id]})
			}
		}
		for _, id := range model.Keys(s.Transitions) {
			if t := s.Transitions[id]; t.Kind != model.KindStart && !reachedTransitions[id] && reached[t.From] {
				r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "unreachable-transition", Severity: "warning", Subject: "transitions/" + id, Message: "transition is never taken from a reachable state (an inner event with the same name overrides it)", Source: s.Sources["transitions/"+id]})
			}
		}
	}
	for _, uid := range model.Keys(s.UseCases) {
		u := s.UseCases[uid]
		scope := map[string]bool{}
		for _, x := range u.Scope {
			scope[x] = true
		}
		ends := map[string]bool{}
		for _, x := range u.Ends {
			ends[x] = true
		}
		g := Explore(e, u.Starts, u.Actor, scope, ends)
		r.Diagnostics = append(r.Diagnostics, g.Diagnostics...)
		// Even an empty scenario set must not conceal a use case that cannot finish.
		for i, start := range g.Starts {
			status, message := "incomplete", "completion reachability not established"
			var witness *model.Witness
			seen := map[int]bool{start: true}
			queue := []int{start}
			for q := 0; q < len(queue); q++ {
				n := queue[q]
				if g.Nodes[n].Terminal {
					status, message = "verified", "some completion path exists from this start"
					witness = copyWitness(g.Nodes[n].Witness)
					break
				}
				for _, edgeID := range g.Out[n] {
					if to := g.Edges[edgeID].To; !seen[to] {
						seen[to] = true
						queue = append(queue, to)
					}
				}
			}
			if status != "verified" && g.Complete {
				status, message = "failed", "no completion path from this use-case start"
				witness = copyWitness(g.Nodes[start].Witness)
			}
			r.Checks = append(r.Checks, model.Check{ID: fmt.Sprintf("completion/%s/%d", uid, i+1), Kind: "usecase-completion", Status: status, Message: message, Witness: witness})
		}
		r.Complete = r.Complete && g.Complete
		for _, start := range u.Starts {
			if v, err := e.Start(start); err == nil && global.Complete && !reached[v.State] {
				r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "unreachable-start", Severity: "error", Subject: "usecases/" + uid, Message: "use-case start " + start + " is not reachable from the root start", Source: s.Sources["usecases/"+uid]})
			}
		}
		scenarios := []model.ScenarioResult{}
		for _, sid := range model.Keys(u.Scenarios) {
			sc := e.Replay(uid+"/"+sid, u.Scenarios[sid], &u)
			sc.UseCase = uid
			scenarios = append(scenarios, sc)
			r.Scenarios = append(r.Scenarios, sc)
			if !sc.Valid {
				r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "scenario", Severity: "error", Subject: "usecases/" + uid, Message: sid + ": " + sc.Error, Source: s.Sources["usecases/"+uid], Witness: copyWitness(sc.Witness)})
			}
		}
		cov := CoverageFor(s, u, g, scenarios)
		leaves := 0
		for _, n := range g.Nodes {
			if s.IsLeaf(n.State.State) {
				leaves++
			}
		}
		ur := model.UseCaseResult{ID: uid, Coverage: cov, ReachableStates: leaves, Complete: g.Complete, ReviewBasisHash: reviewBasisHash(s, uid, version)}
		for _, c := range cov {
			if !c.Complete {
				ur.Complete = false
				r.Complete = false
				r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "coverage-incomplete", Severity: "error", Subject: "usecases/" + uid, Message: c.Kind + " enumeration unfinished"})
			}
			if threshold, ok := s.Project.Coverage[c.Kind]; ok && c.Percent != nil && *c.Percent+0.000001 < threshold {
				r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "coverage-threshold", Severity: "error", Subject: "usecases/" + uid, Message: fmt.Sprintf("%s %.2f%% < %.2f%%", c.Kind, *c.Percent, threshold)})
			}
			if len(c.Infeasible) > 0 {
				r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "infeasible-requirements", Severity: "warning", Subject: "usecases/" + uid, Message: fmt.Sprintf("%s has %d paths that cannot reach a use-case end (listed, not counted)", c.Kind, len(c.Infeasible))})
			}
		}
		ur.CoverageReviews = evaluateCoverageReviews(u, ur, scenarios)
		r.UseCases = append(r.UseCases, ur)
		r.Dependencies[uid] = dependencies(s, uid, g)
	}
	for _, id := range model.Keys(s.Regressions) {
		sc := e.Replay("regressions/"+id, s.Regressions[id], nil)
		r.Scenarios = append(r.Scenarios, sc)
		status := "verified"
		if !sc.Valid {
			status = "failed"
		}
		r.Checks = append(r.Checks, model.Check{ID: id, Kind: "regression", Status: status, Message: sc.Error, Witness: copyWitness(sc.Witness)})
	}
	finish(r)
	return r
}
func hasErrors(ds []model.Diagnostic) bool {
	for _, d := range ds {
		if d.Severity == "error" {
			return true
		}
	}
	return false
}
func finish(r *model.Report) {
	r.Status = "verified"
	if !r.Complete {
		r.Status = "incomplete"
	}
	for _, c := range r.Checks {
		if c.Status == "failed" {
			r.Status = "failed"
		}
		if c.Status == "incomplete" && r.Status == "verified" {
			r.Status = "incomplete"
		}
	}
	for _, d := range r.Diagnostics {
		if d.Severity == "error" && d.Code != "exploration-incomplete" && d.Code != "coverage-incomplete" {
			r.Status = "failed"
		}
	}
	sort.SliceStable(r.Diagnostics, func(i, j int) bool {
		a, b := r.Diagnostics[i], r.Diagnostics[j]
		return a.Subject+"/"+a.Code+"/"+a.Message < b.Subject+"/"+b.Code+"/"+b.Message
	})
}

// definitions hashes every definition so that a baseline report can show what changed.
func definitions(s *model.Spec, r *model.Report) {
	put := func(section string, values map[string]any) {
		for _, k := range model.Keys(values) {
			r.Definitions[section+"/"+k] = model.Hash(values[k])
		}
	}
	put("actors", anyMap(s.Actors))
	put("states", anyMap(s.States))
	put("transitions", anyMap(s.Transitions))
	put("decisions", anyMap(s.Decisions))
	put("usecasePackages", anyMap(s.UseCasePackages))
	put("usecases", anyMap(s.UseCases))
	put("commonRules", anyMap(s.CommonRules))
	put("regressions", anyMap(s.Regressions))
	r.Definitions["project"] = model.Hash(s.Project)
}
func anyMap[V any](m map[string]V) map[string]any {
	r := map[string]any{}
	for k, v := range m {
		r[k] = v
	}
	return r
}

// dependencies lists the definitions a use case's result depends on: every
// state its graph visits (with ancestors), every transition declared there
// (inherited or overriding events change dispatch), and the decisions below.
func dependencies(s *model.Spec, id string, g *Graph) []string {
	u := s.UseCases[id]
	deps := map[string]bool{"project": true, "usecases/" + id: true, "actors/" + u.Actor: true}
	for rule := range s.CommonRules {
		deps["commonRules/"+rule] = true
	}
	for p := u.Package; p != "" && !deps["usecasePackages/"+p]; p = s.UseCasePackages[p].Parent {
		deps["usecasePackages/"+p] = true
	}
	states := map[string]bool{}
	for _, n := range g.Nodes {
		if d, ok := s.Decisions[n.State.State]; ok {
			deps["decisions/"+n.State.State] = true
			for _, path := range s.StatePath(d.Parent) {
				states[path] = true
			}
			continue
		}
		for _, path := range s.StatePath(n.State.State) {
			states[path] = true
		}
	}
	for path := range states {
		deps["states/"+path] = true
	}
	for tid, t := range s.Transitions {
		root := s.Transitions[t.Root]
		if states[root.From] {
			deps["transitions/"+tid] = true
			if t.To != "" && s.IsDecision(t.To) {
				deps["decisions/"+t.To] = true
			}
		}
	}
	return model.Keys(deps)
}

func Compare(now, old *model.Report) error {
	if old.FormatVersion != "1" {
		return fmt.Errorf("unsupported baseline report version")
	}
	if now.Name != old.Name {
		return fmt.Errorf("baseline project name differs")
	}
	d := &model.Impact{Changed: []string{}, AffectedUseCases: []string{}, InvalidatedScenarios: []string{}, CoverageChanges: map[string]float64{}}
	keys := map[string]bool{}
	for k := range now.Definitions {
		keys[k] = true
	}
	for k := range old.Definitions {
		keys[k] = true
	}
	changes := map[string]bool{}
	for _, k := range model.Keys(keys) {
		if now.Definitions[k] != old.Definitions[k] {
			d.Changed = append(d.Changed, k)
			changes[k] = true
		}
	}
	allUC := map[string]bool{}
	for k := range now.Dependencies {
		allUC[k] = true
	}
	for k := range old.Dependencies {
		allUC[k] = true
	}
	for _, id := range model.Keys(allUC) {
		deps := append(append([]string{}, now.Dependencies[id]...), old.Dependencies[id]...)
		for _, k := range deps {
			if changes[k] {
				d.AffectedUseCases = append(d.AffectedUseCases, id)
				break
			}
		}
	}
	prev := map[string]bool{}
	for _, x := range old.Scenarios {
		prev[x.ID] = x.Valid
	}
	current := map[string]bool{}
	for _, x := range now.Scenarios {
		current[x.ID] = x.Valid
	}
	for _, id := range model.Keys(prev) {
		if prev[id] && !current[id] {
			d.InvalidatedScenarios = append(d.InvalidatedScenarios, id)
		}
	}
	pc := map[string]float64{}
	for _, u := range old.UseCases {
		for _, c := range u.Coverage {
			if c.Percent != nil {
				pc[u.ID+"/"+c.Kind] = *c.Percent
			}
		}
	}
	for _, u := range now.UseCases {
		for _, c := range u.Coverage {
			key := u.ID + "/" + c.Kind
			if p, ok := pc[key]; ok && c.Percent != nil && p != *c.Percent {
				d.CoverageChanges[key] = *c.Percent - p
			}
		}
	}
	now.Impact = d
	return nil
}
