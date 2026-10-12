package analysis

import (
	"fmt"

	"github.com/nuitsjp/ucspec/internal/engine"
	"github.com/nuitsjp/ucspec/internal/model"
)

// Node is a control configuration. Ended marks that a use-case end transition
// was taken; a node is terminal once it is ended and back at a leaf.
type Node struct {
	State    model.Snapshot
	Ended    bool
	Witness  model.Witness
	Terminal bool
}
type Edge struct {
	From       int
	To         int
	Transition string
}
type Graph struct {
	Nodes       []Node
	Edges       []Edge
	Out         map[int][]int
	Starts      []int
	Complete    bool
	Diagnostics []model.Diagnostic
}

// Explore builds the control graph from the given start states. actor limits
// the events taken ("" allows all), scope limits events (nil allows all) and
// ends marks the transitions that complete a use case (nil: never complete).
func Explore(e *engine.Engine, starts []string, actor string, scope, ends map[string]bool) *Graph {
	g := &Graph{Out: map[int][]int{}, Complete: true}
	ids := map[string]int{}
	limit := e.Spec.Project.Limits.Paths
	add := func(st model.Snapshot, ended bool, w model.Witness) int {
		key := model.JSON([]any{st, ended})
		if id, ok := ids[key]; ok {
			return id
		}
		if len(g.Nodes) >= limit {
			g.Complete = false
			return -1
		}
		id := len(g.Nodes)
		ids[key] = id
		g.Nodes = append(g.Nodes, Node{State: st, Ended: ended, Witness: w, Terminal: ended && !e.Spec.IsDecision(st.State)})
		return id
	}
	for _, state := range starts {
		v, err := e.Start(state)
		if err != nil {
			g.Complete = false
			g.Diagnostics = append(g.Diagnostics, model.Diagnostic{Code: "start", Severity: "error", Subject: "states/" + state, Message: err.Error()})
			continue
		}
		id := add(v, false, model.Witness{Start: v.State, Transitions: []string{}})
		if id >= 0 && !containsInt(g.Starts, id) {
			g.Starts = append(g.Starts, id)
		}
	}
	for i := 0; i < len(g.Nodes); i++ {
		n := g.Nodes[i]
		if n.Terminal {
			continue
		}
		edges, err := e.Outgoing(n.State, actor)
		if err != nil {
			g.Complete = false
			g.Diagnostics = append(g.Diagnostics, model.Diagnostic{Code: "transition", Severity: "error", Subject: "states/" + n.State.State, Message: err.Error(), Witness: copyWitness(n.Witness)})
			continue
		}
		for _, ed := range edges {
			t := e.Spec.Transitions[ed.Transition]
			if scope != nil && t.Kind == model.KindEvent && !scope[ed.Transition] {
				continue
			}
			to := add(ed.Next, n.Ended || ends[ed.Transition], extend(n.Witness, ed.Transition))
			if to < 0 {
				continue
			}
			j := len(g.Edges)
			g.Edges = append(g.Edges, Edge{From: i, To: to, Transition: ed.Transition})
			g.Out[i] = append(g.Out[i], j)
		}
	}
	if !g.Complete {
		g.Diagnostics = append(g.Diagnostics, model.Diagnostic{Code: "exploration-incomplete", Severity: "error", Message: fmt.Sprintf("exploration incomplete (%d configurations, limit %d); not a proof", len(g.Nodes), limit)})
	}
	return g
}
func copyWitness(w model.Witness) *model.Witness {
	r := w
	r.Transitions = append([]string{}, w.Transitions...)
	return &r
}
func extend(w model.Witness, t string) model.Witness {
	return model.Witness{Start: w.Start, Transitions: append(append([]string{}, w.Transitions...), t)}
}
func containsInt(s []int, n int) bool {
	for _, v := range s {
		if v == n {
			return true
		}
	}
	return false
}

// WitnessFor uses a product of the control graph and a sequence matcher. It
// finds a COMPLETE path containing path, without a depth bound.
// feasible=false is only conclusive when complete=true.
func WitnessFor(g *Graph, path []string, budget int) (w *model.Witness, feasible, complete bool) {
	type product struct {
		Node     int
		Progress int
		W        model.Witness
	}
	queue := []product{}
	seen := map[[2]int]bool{}
	for _, n := range g.Starts {
		k := [2]int{n, 0}
		seen[k] = true
		queue = append(queue, product{Node: n, W: g.Nodes[n].Witness})
	}
	for i := 0; i < len(queue); i++ {
		p := queue[i]
		if p.Progress == len(path) && g.Nodes[p.Node].Terminal {
			return copyWitness(p.W), true, true
		}
		if g.Nodes[p.Node].Terminal {
			continue
		}
		for _, ei := range g.Out[p.Node] {
			ed := g.Edges[ei]
			q := advance(path, p.Progress, ed.Transition)
			k := [2]int{ed.To, q}
			if seen[k] {
				continue
			}
			if len(seen) >= budget {
				return nil, false, false
			}
			seen[k] = true
			queue = append(queue, product{ed.To, q, extend(p.W, ed.Transition)})
		}
	}
	return nil, false, g.Complete
}
func advance(pattern []string, q int, tr string) int {
	if q == len(pattern) {
		return q
	}
	prefix := append(append([]string{}, pattern[:q]...), tr)
	for n := min(len(pattern), len(prefix)); n > 0; n-- {
		ok := true
		for i := 0; i < n; i++ {
			if prefix[len(prefix)-n+i] != pattern[i] {
				ok = false
				break
			}
		}
		if ok {
			return n
		}
	}
	return 0
}
