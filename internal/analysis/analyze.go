package analysis

import (
	"encoding/json"
	"fmt"
	"github.com/nuitsjp/usecase-spec/internal/compiler"
	"github.com/nuitsjp/usecase-spec/internal/engine"
	"github.com/nuitsjp/usecase-spec/internal/model"
	"sort"
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
	global := Explore(e, s.Project.Initial, nil, nil)
	r.Complete = global.Complete
	r.ReachableStates = len(global.Nodes)
	r.Diagnostics = append(r.Diagnostics, global.Diagnostics...)
	reachableStates := map[string]bool{}
	for _, n := range global.Nodes {
		reachableStates[n.State.State] = true
	}
	for _, id := range model.Keys(s.States) {
		if !reachableStates[id] && global.Complete {
			r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "unreachable-state", Severity: "warning", Subject: "states/" + id, Message: "state is unreachable from project.initial", Source: s.Sources["states/"+id]})
		}
	}
	r.Checks = append(r.Checks, Properties(e, global)...)
	for _, uid := range model.Keys(s.UseCases) {
		u := s.UseCases[uid]
		scope := map[string]bool{}
		for _, x := range u.Scope {
			scope[x] = true
		}
		g := Explore(e, u.Starts, scope, u.End)
		// Even an empty scenario set must not conceal a use case that cannot finish.
		// This checks EF end separately for EACH declared start (not AF end).
		for i, start := range g.Starts {
			status := "incomplete"
			message := "completion reachability not established"
			seen := map[int]bool{start: true}
			queue := []int{start}
			var witness *model.Witness
			for q := 0; q < len(queue); q++ {
				n := queue[q]
				if g.Nodes[n].Terminal {
					status = "verified"
					message = "some completion path exists from this start"
					witness = copyWitness(g.Nodes[n].Witness)
					break
				}
				for _, edgeID := range g.Out[n] {
					to := g.Edges[edgeID].To
					if !seen[to] {
						seen[to] = true
						queue = append(queue, to)
					}
				}
			}
			if status != "verified" && g.Complete {
				status = "failed"
				message = "no completion path from this use-case start"
				witness = copyWitness(g.Nodes[start].Witness)
			}
			r.Checks = append(r.Checks, model.Check{ID: fmt.Sprintf("completion/%s/%d", uid, i+1), Kind: "usecase-completion", Status: status, Message: message, Witness: witness})
		}
		r.Complete = r.Complete && g.Complete
		if !g.Complete {
			r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "usecase-incomplete", Severity: "error", Subject: "usecases/" + uid, Message: "use-case execution graph incomplete", Source: s.Sources["usecases/"+uid]})
		}
		// A use-case start must also be a reachable SYSTEM configuration.
		sysStates := map[string]bool{}
		for _, n := range global.Nodes {
			sysStates[model.JSON(n.State)] = true
		}
		for _, st := range u.Starts {
			n, _ := s.Normalize(st)
			if !sysStates[model.JSON(n)] && global.Complete {
				r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "unreachable-start", Severity: "error", Subject: "usecases/" + uid, Message: "use-case start is not reachable in the system model", Source: s.Sources["usecases/"+uid]})
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
		ur := model.UseCaseResult{ID: uid, Coverage: cov, ReachableStates: len(g.Nodes), Complete: g.Complete}
		for _, c := range cov {
			if !c.Complete {
				ur.Complete = false
				r.Complete = false
				r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "coverage-incomplete", Severity: "error", Subject: "usecases/" + uid, Message: c.Kind + " enumeration/feasibility unfinished"})
			}
			if threshold, ok := s.Project.Coverage[c.Kind]; ok && c.Percent != nil && *c.Percent+0.000001 < threshold {
				r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "coverage-threshold", Severity: "error", Subject: "usecases/" + uid, Message: fmt.Sprintf("%s %.2f%% < %.2f%%", c.Kind, *c.Percent, threshold)})
			}
			if len(c.Infeasible) > 0 {
				r.Diagnostics = append(r.Diagnostics, model.Diagnostic{Code: "infeasible-requirements", Severity: "warning", Subject: "usecases/" + uid, Message: fmt.Sprintf("%s has %d infeasible control paths (listed, not silently counted)", c.Kind, len(c.Infeasible))})
			}
		}
		r.UseCases = append(r.UseCases, ur)
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
	for _, id := range model.Keys(s.Decisions) {
		table := s.Decisions[id]
		for i, c := range table.Cases {
			st, err := s.Normalize(model.Snapshot{State: table.State, Values: c.Values})
			status := "verified"
			msg := "matches independent decision table"
			if err != nil {
				status = "failed"
				msg = err.Error()
			} else {
				ids, _, _, er := e.Select(st, table.Event, c.Input)
				expected := sortedStrings(c.Enabled)
				if er != nil || model.JSON(sortedStrings(ids)) != model.JSON(expected) {
					status = "failed"
					msg = fmt.Sprintf("expected enabled %v; got %v (%v)", expected, ids, er)
				}
			}
			r.Checks = append(r.Checks, model.Check{ID: fmt.Sprintf("%s/%d", id, i+1), Kind: "decision", Status: status, Message: msg})
		}
	}
	finish(r)
	return r
}
func Properties(e *engine.Engine, g *Graph) []model.Check {
	result := []model.Check{}
	for _, id := range model.Keys(e.Spec.Properties) {
		p := e.Spec.Properties[id]
		c := model.Check{ID: id, Kind: p.Kind, Status: "verified", Message: p.Description}
		if !g.Complete {
			c.Status = "incomplete"
		}
		truth := make([]bool, len(g.Nodes))
		evalError := false
		for i, n := range g.Nodes {
			v, err := engine.Predicate(e.Spec, n.State, p.Assert)
			if err != nil {
				c.Status = "failed"
				c.Message = err.Error()
				c.Witness = copyWitness(n.Witness)
				evalError = true
				break
			}
			truth[i] = v
		}
		if evalError {
			result = append(result, c)
			continue
		}
		switch p.Kind {
		case "invariant":
			for i, ok := range truth {
				if !ok {
					c.Status = "failed"
					c.Witness = copyWitness(g.Nodes[i].Witness)
					break
				}
			}
		case "reachable":
			found := false
			for i, ok := range truth {
				if ok {
					found = true
					c.Status = "verified"
					c.Witness = copyWitness(g.Nodes[i].Witness)
					break
				}
			}
			if !found && g.Complete {
				c.Status = "failed"
				c.Message += " (no reachable state satisfies predicate)"
			}
		case "recoverable":
			// AG EF target: every reachable configuration has some path to target.
			// This is NOT AF target (inevitable termination under arbitrary scheduling).
			if g.Complete {
				back := map[int][]int{}
				for _, ed := range g.Edges {
					back[ed.To] = append(back[ed.To], ed.From)
				}
				seen := map[int]bool{}
				q := []int{}
				for i, ok := range truth {
					if ok {
						seen[i] = true
						q = append(q, i)
					}
				}
				for i := 0; i < len(q); i++ {
					for _, n := range back[q[i]] {
						if !seen[n] {
							seen[n] = true
							q = append(q, n)
						}
					}
				}
				for i, n := range g.Nodes {
					if !seen[i] {
						c.Status = "failed"
						c.Witness = copyWitness(n.Witness)
						break
					}
				}
			}
		}
		result = append(result, c)
	}
	return result
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
		if d.Severity == "error" && d.Code != "exploration-incomplete" && d.Code != "coverage-incomplete" && d.Code != "usecase-incomplete" && d.Code != "input-limit" {
			r.Status = "failed"
		}
	}
	sort.SliceStable(r.Diagnostics, func(i, j int) bool {
		a, b := r.Diagnostics[i], r.Diagnostics[j]
		return a.Subject+"/"+a.Code+"/"+a.Message < b.Subject+"/"+b.Code+"/"+b.Message
	})
}
func definitions(s *model.Spec, r *model.Report) {
	put := func(section string, values any) {
		raw := map[string]any{}
		b := []byte(model.JSON(values))
		_ = jsonUnmarshal(b, &raw)
		for _, k := range model.Keys(raw) {
			r.Definitions[section+"/"+k] = model.Hash(raw[k])
		}
	}
	put("states", s.States)
	put("screens", s.Screens)
	put("events", s.Events)
	put("conditions", s.Conditions)
	put("variables", s.Variables)
	put("transitions", s.Transitions)
	put("usecases", s.UseCases)
	put("properties", s.Properties)
	put("decisions", s.Decisions)
	put("regressions", s.Regressions)
	r.Definitions["project"] = model.Hash(s.Project)
	// Conservative transitive dependency closure: conditions and state variables may
	// affect any path. Prefer over-reporting impact to silently missing an effect.
	for _, id := range model.Keys(s.UseCases) {
		u := s.UseCases[id]
		deps := map[string]bool{"project": true, "usecases/" + id: true}
		for _, tr := range u.Scope {
			t := s.Transitions[tr]
			deps["transitions/"+tr] = true
			deps["states/"+t.From] = true
			deps["states/"+t.To] = true
			deps["screens/"+s.States[t.From].Screen] = true
			deps["screens/"+s.States[t.To].Screen] = true
			deps["events/"+t.Event] = true
		}
		for k := range s.Conditions {
			deps["conditions/"+k] = true
		}
		for k := range s.Variables {
			deps["variables/"+k] = true
		}
		r.Dependencies[id] = model.Keys(deps)
	}
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

// Keep a tiny JSON bridge here rather than reflect-dependent semantic hashes.
func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
