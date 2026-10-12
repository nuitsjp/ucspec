package analysis

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nuitsjp/ucspec/internal/model"
)

func pathKey(p []string) string { return strings.Join(p, " > ") }
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

// ScopeTransitions lists the transitions a use case is responsible for: the
// events in its scope, every branch below them, and the branches of start
// decisions that its graph passes through.
func ScopeTransitions(s *model.Spec, u model.UseCase, g *Graph) []string {
	set := map[string]bool{}
	for _, id := range u.Scope {
		set[id] = true
	}
	for id, t := range s.Transitions {
		if t.Kind == model.KindBranch && set[t.Root] {
			set[id] = true
		}
	}
	for _, ed := range g.Edges {
		if t := s.Transitions[ed.Transition]; t.Kind == model.KindBranch && s.Transitions[t.Root].Kind == model.KindStart {
			set[ed.Transition] = true
		}
	}
	return model.Keys(set)
}

func CoverageFor(s *model.Spec, u model.UseCase, g *Graph, sc []model.ScenarioResult) []model.Coverage {
	paths := [][]string{}
	for _, r := range sc {
		if r.Valid {
			paths = append(paths, r.Witness.Transitions)
		}
	}
	required := map[string]bool{}
	raw := map[string][][]string{"transitions": {}, "pairs": {}}
	for _, id := range ScopeTransitions(s, u, g) {
		required[id] = true
		raw["transitions"] = append(raw["transitions"], []string{id})
	}
	pairs := map[string][]string{}
	for _, in := range g.Edges {
		for _, j := range g.Out[in.To] {
			out := g.Edges[j]
			if required[in.Transition] && required[out.Transition] {
				p := []string{in.Transition, out.Transition}
				pairs[pathKey(p)] = p
			}
		}
	}
	for _, k := range model.Keys(pairs) {
		raw["pairs"] = append(raw["pairs"], pairs[k])
	}
	prime, primeComplete := PrimePaths(g, s.Project.Limits.Paths)
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
		c := coverage(kind, req, covered, complete, "Transitions are events and decision branches. Paths that cannot reach a use-case end are listed separately and not counted. Pairs have exactly two transitions.")
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
	results = append(results, coverage("boundedPaths", bounded, covered, bc, fmt.Sprintf("Complete paths from a start to a use-case end with up to %d transitions.", s.Project.Limits.Steps)))
	return results
}

// StateCoverage counts reachable leaf and composite states. A reached composite
// does not cover its other children.
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
			if id == "" || s.IsLeaf(id) == (kind == "compositeStates") {
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
		note := "Reachable leaf states in the use-case graph. Only valid scenario traces cover a state."
		if kind == "compositeStates" {
			note = "Reachable composite states, counted separately from leaves. A parent is active with its child; parent coverage does not cover other children."
		}
		results = append(results, coverage(kind, req, covered, complete, note))
	}
	return results
}

// StateWitness finds a completing path that visits a leaf or composite.
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
		queue = append(queue, product{n, visited, g.Nodes[n].Witness})
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
			queue = append(queue, product{ed.To, visited, extend(p.w, ed.Transition)})
		}
	}
	return nil, g.Complete
}

// PrimePaths enumerates maximal simple paths of the graph by transition ID. A
// cycle may repeat ONLY its first vertex at its end.
func PrimePaths(g *Graph, budget int) ([][]string, bool) {
	all := map[string][]string{}
	work := 0
	complete := true
	var dfs func(start, at int, seen map[int]bool, path []string)
	dfs = func(start, at int, seen map[int]bool, path []string) {
		if !complete {
			return
		}
		work++
		if work > budget {
			complete = false
			return
		}
		if len(path) > 0 {
			all[pathKey(path)] = append([]string{}, path...)
		}
		for _, ei := range g.Out[at] {
			edge := g.Edges[ei]
			p := append(append([]string{}, path...), edge.Transition)
			if edge.To == start {
				all[pathKey(p)] = p
				continue
			}
			if seen[edge.To] {
				continue
			}
			seen[edge.To] = true
			dfs(start, edge.To, seen, p)
			delete(seen, edge.To)
		}
	}
	for v := range g.Nodes {
		dfs(v, v, map[int]bool{v: true}, nil)
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

// Bounded enumerates complete paths from a start to a use-case end.
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
			k := pathKey(w.Transitions)
			if _, ok := req[k]; !ok {
				req[k] = model.Requirement{ID: k, Path: append([]string{}, w.Transitions...), Witness: copyWitness(w)}
			}
			return
		}
		if len(w.Transitions) >= maxSteps {
			return
		}
		for _, i := range g.Out[n] {
			if work >= budget {
				complete = false
				return
			}
			ed := g.Edges[i]
			visit(ed.To, extend(w, ed.Transition))
		}
	}
	for _, n := range g.Starts {
		if work >= budget {
			complete = false
			break
		}
		visit(n, g.Nodes[n].Witness)
	}
	return req, complete
}
