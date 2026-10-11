package analysis

import (
	"fmt"
	"github.com/nuitsjp/ucspec/internal/model"
	"sort"
	"strings"
)

func pathKey(p []string) string { return strings.Join(p, " > ") }
func idsOf(steps []model.Step) []string {
	r := []string{}
	for _, s := range steps {
		if !s.Reject {
			r = append(r, s.Transition)
		}
	}
	return r
}
func containsPath(haystack, needle []string) bool {
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		ok := true
		for j := range needle {
			if needle[j] != haystack[i+j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
func coverage(kind string, req map[string]model.Requirement, covered map[string]bool, complete bool, note string) model.Coverage {
	c := model.Coverage{Kind: kind, Total: len(req), Complete: complete, Missing: []model.Requirement{}, Note: note}
	for _, k := range model.Keys(req) {
		if covered[k] {
			c.Covered++
		} else {
			c.Missing = append(c.Missing, req[k])
		}
	}
	if c.Total > 0 {
		p := float64(c.Covered) * 100 / float64(c.Total)
		c.Percent = &p
	}
	return c
}
func CoverageFor(s *model.Spec, u model.UseCase, g *Graph, sc []model.ScenarioResult) []model.Coverage {
	paths := [][]string{}
	observed := []model.Observation{}
	for _, r := range sc {
		if r.Valid {
			paths = append(paths, idsOf(r.Witness.Steps))
			observed = append(observed, r.GuardObservations...)
		}
	}
	scope := map[string]bool{}
	for _, id := range u.Scope {
		scope[id] = true
	}
	raw := map[string][][]string{"transitions": {}, "pairs": {}}
	for _, a := range model.Keys(scope) {
		raw["transitions"] = append(raw["transitions"], []string{a})
		after := transitionTargets(s, s.Transitions[a])
		for _, b := range model.Keys(scope) {
			for _, state := range after {
				if s.InState(state, s.Transitions[b].From) {
					raw["pairs"] = append(raw["pairs"], []string{a, b})
					break
				}
			}
		}
	}
	prime, primeComplete := PrimePaths(s, scope, s.Project.Limits.Paths)
	raw["primePaths"] = prime
	results := StateCoverage(s, g, sc)
	for _, kind := range []string{"transitions", "pairs", "primePaths"} {
		req := map[string]model.Requirement{}
		infeasible := []model.Requirement{}
		covered := map[string]bool{}
		complete := g.Complete
		if kind == "primePaths" {
			complete = complete && primeComplete
		}
		for _, p := range raw[kind] {
			key := pathKey(p)
			w, feasible, done := WitnessFor(g, p, s.Project.Limits.Paths)
			r := model.Requirement{ID: key, Path: p, Witness: w}
			if done && !feasible {
				infeasible = append(infeasible, r)
				continue
			}
			if !done {
				complete = false
			}
			req[key] = r
			for _, path := range paths {
				if containsPath(path, p) {
					covered[key] = true
					break
				}
			}
		}
		c := coverage(kind, req, covered, complete, "Control-state paths; data feasibility checked in the finite execution graph. Infeasible requirements are listed separately. Pairs have exactly two transitions.")
		c.Infeasible = infeasible
		results = append(results, c)
	}
	bounded, bc := Bounded(g, s.Project.Limits.Steps, s.Project.Limits.Paths)
	covered := map[string]bool{}
	for _, p := range paths {
		if len(p) <= s.Project.Limits.Steps {
			covered[pathKey(p)] = true
		}
	}
	results = append(results, coverage("boundedPaths", bounded, covered, bc, fmt.Sprintf("Complete transition-ID paths up to %d transitions; data variants are merged here and checked by condition metrics separately.", s.Project.Limits.Steps)))
	for _, kind := range []string{"conditions", "multipleConditions"} {
		req := observationRequirements(g.Observations, scope, kind)
		seen := observationRequirements(observed, scope, kind)
		covered := map[string]bool{}
		for k := range seen {
			covered[k] = true
		}
		c := coverage(kind, req, covered, g.Complete, "Logical truth of named conditions, at each transition guard evaluation (including false guards). Denominator: reachable state/event/input combinations, not arbitrary Boolean combinations; not runtime short-circuit coverage.")
		results = append(results, c)
	}
	return results
}

// StateCoverage counts reachable control states, independent of data variants.
// A reached composite does not imply that any other child has been covered.
func StateCoverage(s *model.Spec, g *Graph, scenarios []model.ScenarioResult) []model.Coverage {
	covered := map[string]bool{}
	for _, scenario := range scenarios {
		if !scenario.Valid {
			continue
		}
		for _, st := range scenario.Trace {
			for _, id := range s.StatePath(st.State) {
				covered[id] = true
			}
		}
	}
	reachable := map[string]bool{}
	for _, n := range g.Nodes {
		for _, id := range s.StatePath(n.State.State) {
			reachable[id] = true
		}
	}
	results := []model.Coverage{}
	for _, kind := range []string{"states", "compositeStates"} {
		req := map[string]model.Requirement{}
		complete := g.Complete
		for _, id := range model.Keys(reachable) {
			if s.IsChoice(id) {
				continue
			}
			if (s.States[id].Initial != "") != (kind == "compositeStates") {
				continue
			}
			r := model.Requirement{ID: id, State: id}
			if !covered[id] {
				var done bool
				r.Witness, done = StateWitness(s, g, id, s.Project.Limits.Paths)
				complete = complete && done
			}
			req[id] = r
		}
		note := "Reachable leaf states in the use-case execution graph; data variants count once. Only valid scenario traces cover a state."
		if kind == "compositeStates" {
			note = "Reachable composite states, counted separately from leaves. A parent is active with its child; parent coverage does not cover other children."
		}
		results = append(results, coverage(kind, req, covered, complete, note))
	}
	return results
}

// StateWitness finds a completing scenario that visits a leaf or composite.
// A missing completion witness does not remove a reachable state from coverage.
func StateWitness(s *model.Spec, g *Graph, target string, budget int) (*model.Witness, bool) {
	type product struct {
		node    int
		visited bool
		w       model.Witness
	}
	queue := []product{}
	seen := map[[2]int]bool{}
	key := func(node int, visited bool) [2]int {
		v := 0
		if visited {
			v = 1
		}
		return [2]int{node, v}
	}
	for _, n := range g.Starts {
		visited := s.InState(g.Nodes[n].State.State, target)
		seen[key(n, visited)] = true
		queue = append(queue, product{n, visited, model.Witness{Start: g.Nodes[n].State, Steps: []model.Step{}}})
	}
	for i := 0; i < len(queue); i++ {
		p := queue[i]
		if g.Nodes[p.node].Terminal {
			if p.visited {
				return copyWitness(p.w), true
			}
			continue
		}
		for _, ei := range g.Out[p.node] {
			ed := g.Edges[ei]
			visited := p.visited || s.InState(g.Nodes[ed.To].State.State, target)
			k := key(ed.To, visited)
			if seen[k] {
				continue
			}
			if len(seen) >= budget {
				return nil, false
			}
			seen[k] = true
			queue = append(queue, product{ed.To, visited, extend(p.w, ed.Step)})
		}
	}
	return nil, g.Complete
}
func observationRequirements(obs []model.Observation, scope map[string]bool, kind string) map[string]model.Requirement {
	r := map[string]model.Requirement{}
	for _, o := range obs {
		if !scope[o.Transition] || len(o.Atoms) == 0 {
			continue
		}
		if kind == "conditions" {
			for _, a := range model.Keys(o.Atoms) {
				k := fmt.Sprintf("%s / %s=%t", o.Transition, a, o.Atoms[a])
				r[k] = model.Requirement{ID: k}
			}
		} else {
			parts := []string{}
			for _, a := range model.Keys(o.Atoms) {
				parts = append(parts, fmt.Sprintf("%s=%t", a, o.Atoms[a]))
			}
			k := o.Transition + " / " + strings.Join(parts, ", ")
			r[k] = model.Requirement{ID: k}
		}
	}
	return r
}

// PrimePaths enumerates maximal simple paths. A cycle may repeat ONLY its first
// vertex at its end. Distinct rotations of a cycle remain distinct requirements.
func PrimePaths(s *model.Spec, scope map[string]bool, budget int) ([][]string, bool) {
	type arc struct{ id, to string }
	out := map[string][]arc{}
	vertices := map[string]bool{}
	for _, id := range model.Keys(scope) {
		t := s.Transitions[id]
		targets := transitionTargets(s, t)
		for _, from := range s.Leaves(t.From) {
			if t.Kind == "internal" {
				targets = []string{from}
			}
			for _, to := range targets {
				out[from] = append(out[from], arc{id, to})
			}
			vertices[from] = true
		}
		for _, to := range transitionTargets(s, t) {
			vertices[to] = true
		}
	}
	all := map[string][]string{}
	work := 0
	complete := true
	var dfs func(string, string, map[string]bool, []string)
	dfs = func(start, at string, seen map[string]bool, path []string) {
		if !complete {
			return
		}
		work++
		if work > budget {
			complete = false
			return
		}
		if len(path) > 0 {
			key := pathKey(path)
			all[key] = append([]string{}, path...)
		}
		for _, edge := range out[at] {
			to := edge.to
			p := append(append([]string{}, path...), edge.id)
			if to == start {
				all[pathKey(p)] = p
				continue
			}
			if seen[to] {
				continue
			}
			seen[to] = true
			dfs(start, to, seen, p)
			delete(seen, to)
		}
	}
	for _, v := range model.Keys(vertices) {
		dfs(v, v, map[string]bool{v: true}, nil)
		if !complete {
			break
		}
	}
	paths := [][]string{}
	for _, k := range model.Keys(all) {
		paths = append(paths, all[k])
	}
	sort.Slice(paths, func(i, j int) bool {
		if len(paths[i]) != len(paths[j]) {
			return len(paths[i]) > len(paths[j])
		}
		return pathKey(paths[i]) < pathKey(paths[j])
	})
	prime := [][]string{}
	comparisons := 0
	for _, p := range paths {
		contained := false
		for _, q := range prime {
			comparisons++
			if comparisons > budget*10 {
				complete = false
				break
			}
			if len(q) > len(p) && containsPath(q, p) {
				contained = true
				break
			}
		}
		if !contained {
			prime = append(prime, p)
		}
		if comparisons > budget*10 {
			break
		}
	}
	sort.Slice(prime, func(i, j int) bool { return pathKey(prime[i]) < pathKey(prime[j]) })
	return prime, complete
}
func Bounded(g *Graph, maxSteps, budget int) (map[string]model.Requirement, bool) {
	req := map[string]model.Requirement{}
	work := 0
	complete := g.Complete
	var visit func(int, model.Witness)
	visit = func(n int, w model.Witness) {
		work++
		if work > budget {
			complete = false
			return
		}
		if g.Nodes[n].Terminal {
			p := idsOf(w.Steps)
			k := pathKey(p)
			if _, ok := req[k]; !ok {
				req[k] = model.Requirement{ID: k, Path: p, Witness: copyWitness(w)}
			}
			return
		}
		if len(w.Steps) >= maxSteps {
			return
		}
		for _, i := range g.Out[n] {
			if work >= budget {
				complete = false
				return
			}
			ed := g.Edges[i]
			visit(ed.To, extend(w, ed.Step))
		}
	}
	for _, n := range g.Starts {
		if work >= budget {
			complete = false
			break
		}
		visit(n, model.Witness{Start: g.Nodes[n].State, Steps: []model.Step{}})
	}
	return req, complete
}
