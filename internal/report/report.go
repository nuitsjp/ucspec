// Package report renders already-computed analysis. It never explores a model.
package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/nuitsjp/ucspec/internal/model"
	"html"
	"html/template"
	"io/fs"
	"math"
	"sort"
	"strings"
)

type Layout func([]byte) ([]byte, error)
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type Label struct {
	Text   string  `json:"text"`
	X      float64 `json:"x,omitempty"`
	Y      float64 `json:"y,omitempty"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}
type Shape struct {
	ID       string         `json:"id"`
	X        float64        `json:"x,omitempty"`
	Y        float64        `json:"y,omitempty"`
	Width    float64        `json:"width"`
	Height   float64        `json:"height"`
	Options  map[string]any `json:"layoutOptions,omitempty"`
	Children []Shape        `json:"children,omitempty"`
	Edges    []Connection   `json:"edges,omitempty"`
}
type Section struct {
	Start Point   `json:"startPoint"`
	End   Point   `json:"endPoint"`
	Bends []Point `json:"bendPoints,omitempty"`
}
type Connection struct {
	ID        string         `json:"id"`
	Sources   []string       `json:"sources"`
	Targets   []string       `json:"targets"`
	Labels    []Label        `json:"labels"`
	Options   map[string]any `json:"layoutOptions,omitempty"`
	Sections  []Section      `json:"sections,omitempty"`
	Container string         `json:"container,omitempty"`
}
type Graph struct {
	ID       string         `json:"id"`
	Width    float64        `json:"width,omitempty"`
	Height   float64        `json:"height,omitempty"`
	Options  map[string]any `json:"layoutOptions,omitempty"`
	Children []Shape        `json:"children"`
	Edges    []Connection   `json:"edges"`
}

// graphFor builds the semantic graph consumed by the layout engine. Decisions
// are nodes, while their branches remain ordinary transition IDs so that layout
// and coverage share the same identity. Start transitions are drawn from the
// initial markers inside composites; outer markers point at diagram starts.
func graphFor(s *model.Spec, scope, visible []string, starts map[string]string) Graph {
	g := Graph{ID: "root", Options: map[string]any{
		"elk.algorithm": "layered", "elk.direction": "DOWN",
		"elk.layered.nodePlacement.strategy": "NETWORK_SIMPLEX",
		"elk.hierarchyHandling":              "INCLUDE_CHILDREN", "elk.randomSeed": 1,
		"elk.separateConnectedComponents": false,
		"elk.edgeRouting":                 "ORTHOGONAL", "elk.spacing.nodeNode": 45,
		"elk.layered.spacing.nodeNodeBetweenLayers": 100,
		"elk.padding": "[top=45,left=45,bottom=45,right=45]",
	}}
	states := map[string]bool{}
	decisions := map[string]bool{}
	addVertex := func(id string) {
		if id == "" {
			return
		}
		if s.IsDecision(id) {
			decisions[id] = true
		} else {
			states[id] = true
		}
	}
	// States of every scoped transition keep a stable layout across visibility
	// modes; decisions appear only with a visible transition that touches them.
	for _, id := range scope {
		t := s.Transitions[id]
		for _, v := range []string{t.From, drawTarget(s, t)} {
			if !s.IsDecision(v) {
				addVertex(v)
			}
		}
	}
	for _, id := range visible {
		t := s.Transitions[id]
		addVertex(t.From)
		addVertex(drawTarget(s, t))
	}
	for _, target := range starts {
		addVertex(target)
	}
	// Include containers and start trees needed to make a diagram intelligible
	// without adding unrelated events.
	for changed := true; changed; {
		changed = false
		for _, id := range model.Keys(states) {
			st, ok := s.States[id]
			if !ok {
				continue
			}
			needed := []string{}
			if st.Parent != "" {
				needed = append(needed, st.Parent)
			}
			if st.Start != "" {
				needed = append(needed, startVertices(s, st.Start)...)
			}
			for _, v := range needed {
				if !states[v] && !decisions[v] {
					addVertex(v)
					changed = true
				}
			}
		}
		for _, id := range model.Keys(decisions) {
			if parent := s.Decisions[id].Parent; parent != "" && !states[parent] {
				states[parent] = true
				changed = true
			}
		}
	}
	for _, id := range model.Keys(starts) {
		g.Children = append(g.Children, initialShape("__start_"+id))
		g.Edges = append(g.Edges, Connection{ID: "__start_edge_" + id, Sources: []string{"__start_" + id}, Targets: []string{starts[id]}, Labels: actionLabels(s, startActionOf(s, id))})
	}
	var shapeFor func(string) Shape
	shapeFor = func(id string) Shape {
		if d, ok := s.Decisions[id]; ok {
			label := s.Label(id)
			w := math.Max(82, math.Min(250, measure(label, 12)+34))
			lines := wrapText(label, w-24, 12)
			h := math.Max(72, float64(len(lines))*16+30)
			_ = d
			return Shape{ID: id, Width: w, Height: h}
		}
		st := s.States[id]
		w := math.Max(195, math.Min(350, measure(st.Name, 14)+40))
		height := nodeHeader(st, w) + internalHeight(s, visible, id, w)
		n := Shape{ID: id, Width: w, Height: math.Max(76, height+12)}
		if st.Start != "" {
			start := s.Transitions[st.Start]
			n.Options = map[string]any{"elk.direction": "DOWN", "elk.layered.nodePlacement.strategy": "NETWORK_SIMPLEX", "elk.separateConnectedComponents": false, "elk.padding": fmt.Sprintf("[top=%.0f,left=35,bottom=35,right=35]", height+20)}
			n.Children = append(n.Children, Shape{ID: "__initial_" + id, Width: 20, Height: 20})
			n.Edges = append(n.Edges, Connection{ID: st.Start, Sources: []string{"__initial_" + id}, Targets: []string{start.To}, Labels: actionLabels(s, start.Action)})
		}
		for _, child := range graphChildren(s, states, decisions, id) {
			n.Children = append(n.Children, shapeFor(child))
		}
		if st.LayoutLayer != nil {
			if n.Options == nil {
				n.Options = map[string]any{}
			}
			n.Options["elk.partitioning.partition"] = *st.LayoutLayer
		}
		if hasLayoutLayers(n.Children) {
			n.Options["elk.partitioning.activate"] = true
			// elk-go imports INCLUDE_CHILDREN partitions only on the root
			// graph. Explicit ordering edges keep nested bands constrained
			// even when every business transition is hidden.
			bands := map[int][]string{}
			for _, child := range n.Children {
				if layer, ok := child.Options["elk.partitioning.partition"].(int); ok {
					bands[layer] = append(bands[layer], child.ID)
				}
			}
			layers := []int{}
			for layer := range bands {
				layers = append(layers, layer)
			}
			sort.Ints(layers)
			for i := 1; i < len(layers); i++ {
				for _, from := range bands[layers[i-1]] {
					for _, to := range bands[layers[i]] {
						n.Edges = append(n.Edges, Connection{ID: fmt.Sprintf("__layout_layer_edge_%s_%d", id, len(n.Edges)), Sources: []string{from}, Targets: []string{to}, Labels: []Label{}, Options: map[string]any{"elk.layered.priority.direction": 1000}})
					}
				}
			}
		}
		return n
	}
	for _, id := range graphChildren(s, states, decisions, "") {
		g.Children = append(g.Children, shapeFor(id))
	}
	if hasLayoutLayers(g.Children) {
		g.Options["elk.partitioning.activate"] = true
	}
	for _, id := range sorted(visible) {
		t := s.Transitions[id]
		target := drawTarget(s, t)
		present := func(id string) bool { return states[id] || decisions[id] }
		if t.Kind == model.KindStart || target == "" || !present(t.From) || !present(target) {
			continue
		}
		edge := Connection{ID: id, Sources: []string{t.From}, Targets: []string{target}, Labels: labelsFor(transitionLabelLines(s, t))}
		// Keep an event's forward path when automatic cycle breaking chooses
		// between entering a decision and a branch back to the source.
		if t.Kind == model.KindEvent {
			edge.Options = map[string]any{"elk.layered.priority.direction": 10}
		}
		g.Edges = append(g.Edges, edge)
	}
	return g
}

// drawTarget is where an arrow ends. An event without a destination is drawn
// inside its state; a branch without one returns to the state of its event.
func drawTarget(s *model.Spec, t model.Transition) string {
	if t.To != "" {
		return t.To
	}
	if t.Kind == model.KindBranch {
		return s.Transitions[t.Root].From
	}
	return ""
}

// startVertices are the vertices a start transition leads through.
func startVertices(s *model.Spec, id string) []string {
	t := s.Transitions[id]
	if !s.IsDecision(t.To) {
		return []string{t.To}
	}
	r := []string{t.To}
	for _, b := range s.Decisions[t.To].Branches {
		r = append(r, startVertices(s, b)...)
	}
	return r
}

// DiagramTransitions lists the transitions shown for a scope of events: the
// events, every branch below them, and the branches of start decisions.
func DiagramTransitions(s *model.Spec, scope []string) []string {
	set := map[string]bool{}
	all := scope == nil
	for _, id := range scope {
		set[id] = true
	}
	for _, id := range model.Keys(s.Transitions) {
		t := s.Transitions[id]
		switch {
		case t.Kind == model.KindStart:
		case t.Kind == model.KindBranch && s.Transitions[t.Root].Kind == model.KindStart:
			set[id] = true
		case all || set[t.Root]:
			set[id] = true
		}
	}
	return model.Keys(set)
}

func startActionOf(s *model.Spec, marker string) string {
	if marker == "root" {
		return s.Transitions[s.States[""].Start].Action
	}
	return ""
}
func actionLabels(s *model.Spec, action string) []Label {
	if action == "" {
		return []Label{}
	}
	return labelsFor(wrapText("/ "+action, 280, 11))
}
func labelsFor(lines []string) []Label {
	if len(lines) == 0 {
		return []Label{}
	}
	w := 0.0
	for _, line := range lines {
		w = math.Max(w, measure(line, 11))
	}
	return []Label{{Text: strings.Join(lines, "\n"), Width: w + 12, Height: float64(len(lines))*14 + 4}}
}

func initialShape(id string) Shape {
	return Shape{ID: id, Width: 20, Height: 20, Options: map[string]any{"elk.layered.layering.layerConstraint": "FIRST_SEPARATE"}}
}

func hasLayoutLayers(children []Shape) bool {
	for _, child := range children {
		if _, ok := child.Options["elk.partitioning.partition"]; ok {
			return true
		}
	}
	return false
}

// Center the decorative start row without moving any semantic node or route.
// Only the first segment of each start arrow is joined to its moved marker.
func centerStartMarkers(g *Graph) {
	markers := []int{}
	top := g.Height
	width := 0.0
	for i, node := range g.Children {
		if strings.HasPrefix(node.ID, "__start_") {
			markers = append(markers, i)
			width += node.Width
		} else {
			top = math.Min(top, node.Y)
		}
	}
	if len(markers) == 0 {
		return
	}
	const gap = 45.0
	width += float64(len(markers)-1) * gap
	x := (g.Width + 20 - width) / 2
	shifts := map[string]float64{}
	for _, i := range markers {
		marker := &g.Children[i]
		shifts[strings.TrimPrefix(marker.ID, "__start_")] = x - marker.X
		marker.X = x
		x += marker.Width + gap
	}
	for i := range g.Edges {
		edge := &g.Edges[i]
		state, start := strings.CutPrefix(edge.ID, "__start_edge_")
		shift, ok := shifts[state]
		if !start || !ok || len(edge.Sections) == 0 {
			continue
		}
		section := &edge.Sections[0]
		old := section.Start
		section.Start.X += shift
		next := section.End
		if len(section.Bends) > 0 {
			next = section.Bends[0]
		}
		// ELK may start a nominally vertical route with a sub-pixel offset when
		// the target is nested; treat it as vertical and align to the next point.
		if section.Start.X != next.X && math.Abs(next.X-old.X) < 1 && next.Y > old.Y {
			// Turn in the empty space above all top-level states; preserve the
			// remainder of ELK's route, including entries into nested states.
			y := math.Min((old.Y+next.Y)/2, (old.Y+top)/2)
			section.Bends = append([]Point{{section.Start.X, y}, {next.X, y}}, section.Bends...)
		}
	}
}

func internalTransitions(s *model.Spec, scope []string, state string) []string {
	ids := []string{}
	for _, id := range sorted(scope) {
		t := s.Transitions[id]
		if t.Kind == model.KindEvent && t.To == "" && t.From == state {
			ids = append(ids, id)
		}
	}
	return ids
}

func internalLines(s *model.Spec, t model.Transition, width float64) []string {
	return wrapText(strings.Join(transitionLabelLines(s, t), " "), width-30, 11)
}

func internalHeight(s *model.Spec, scope []string, state string, width float64) float64 {
	height := 0.0
	for _, id := range internalTransitions(s, scope, state) {
		height += float64(len(internalLines(s, s.Transitions[id], width)))*14 + 8
	}
	return height
}

func graphChildren(s *model.Spec, states, decisions map[string]bool, parent string) []string {
	children := []string{}
	for _, id := range model.Keys(states) {
		if s.States[id].Parent == parent {
			children = append(children, id)
		}
	}
	for _, id := range model.Keys(decisions) {
		if s.Decisions[id].Parent == parent {
			children = append(children, id)
		}
	}
	return sorted(children)
}

// transitionLabelLines renders "event", "[branch]" and "/ action" lines.
func transitionLabelLines(s *model.Spec, t model.Transition) []string {
	lines := []string{}
	switch t.Kind {
	case model.KindEvent:
		lines = append(lines, wrapText(t.Name, 280, 11)...)
	case model.KindBranch:
		lines = append(lines, wrapText("["+t.Name+"]", 280, 11)...)
	}
	if t.Action != "" {
		lines = append(lines, wrapText("/ "+t.Action, 280, 11)...)
	}
	return lines
}

func transitionTitle(s *model.Spec, id string, t model.Transition) string {
	parts := []string{id}
	if t.Kind == model.KindBranch {
		parts = append(parts, "ディシジョン: "+s.Label(t.From))
	}
	if label := strings.Join(transitionLabelLines(s, t), " "); label != "" {
		parts = append(parts, label)
	}
	return strings.Join(parts, " ")
}

// screenLine is shown under a state name only when the screen differs from it.
func screenLine(st model.StateNode) string {
	if st.Screen == "" || st.Screen == st.Name {
		return ""
	}
	return "画面: " + st.Screen
}

func nodeHeader(st model.StateNode, width float64) float64 {
	h := 30 + float64(len(wrapText(st.Name, width-30, 14)))*18
	if line := screenLine(st); line != "" {
		h += float64(len(wrapText(line, width-30, 11))) * 13
	}
	return h
}
func sorted(ids []string) []string {
	m := map[string]bool{}
	for _, x := range ids {
		m[x] = true
	}
	return model.Keys(m)
}
func wrapText(s string, width, size float64) []string {
	lines := []string{}
	line := ""
	used := 0.0
	for _, r := range s {
		if r == '\n' {
			lines = append(lines, line)
			line = ""
			used = 0
			continue
		}
		w := measure(string(r), size)
		if used+w > width && line != "" {
			lines = append(lines, line)
			line = ""
			used = 0
		}
		line += string(r)
		used += w
	}
	return append(lines, line)
}
func measure(s string, size float64) float64 {
	n := 0.0
	for _, r := range s {
		if r < 128 {
			n += .6
		} else {
			n += 1
		}
	}
	return n * size
}

// draw lays out and renders one diagram. starts maps an outer start marker to
// the vertex it enters.
func draw(s *model.Spec, scope, visible []string, starts map[string]string, layout Layout) (string, error) {
	g := graphFor(s, scope, visible, starts)
	b, err := json.Marshal(g)
	if err != nil {
		return "", err
	}
	if layout == nil {
		return "", fmt.Errorf("layout engine is required")
	}
	b, err = layout(b)
	if err != nil {
		return "", err
	}
	if err = json.Unmarshal(b, &g); err != nil {
		return "", err
	}
	if !finite(g.Width) || !finite(g.Height) || g.Width <= 0 || g.Height <= 0 {
		return "", fmt.Errorf("layout returned invalid graph bounds")
	}
	centerStartMarkers(&g)
	var out strings.Builder
	fmt.Fprintf(&out, `<svg xmlns="http://www.w3.org/2000/svg" role="img" aria-label="状態遷移図" viewBox="0 0 %.2f %.2f"><defs><marker id="arrow" markerWidth="8" markerHeight="8" refX="7" refY="3" orient="auto"><path d="M0,0 L7,3 L0,6" fill="none" stroke="#728795"/></marker></defs>`, g.Width+20, g.Height+20)
	offsets := map[string]Point{"root": {}}
	type placedEdge struct {
		connection Connection
		offset     Point
	}
	edges := []placedEdge{}
	var place func([]Shape, []Connection, Point)
	place = func(nodes []Shape, connections []Connection, parent Point) {
		for _, ed := range connections {
			edges = append(edges, placedEdge{ed, parent})
		}
		for _, n := range nodes {
			p := Point{parent.X + n.X, parent.Y + n.Y}
			offsets[n.ID] = p
			place(n.Children, n.Edges, p)
		}
	}
	place(g.Children, g.Edges, Point{})
	var nodes func([]Shape) error
	nodes = func(children []Shape) error {
		for _, n := range children {
			if !finite(n.X) || !finite(n.Y) || !finite(n.Width) || !finite(n.Height) || n.Width <= 0 || n.Height <= 0 {
				return fmt.Errorf("invalid node bounds")
			}
			if marker, start := strings.CutPrefix(n.ID, "__start_"); start {
				title := "開始: " + s.Label(starts[marker])
				fmt.Fprintf(&out, `<g class="start-node" data-start-state="%s" role="img" aria-label="%s" transform="translate(%.2f %.2f)"><title>%s</title><circle cx="%.2f" cy="%.2f" r="%.2f"/></g>`, esc(starts[marker]), esc(title), n.X, n.Y, esc(title), n.Width/2, n.Height/2, math.Min(n.Width, n.Height)/2)
				continue
			}
			if parent, initial := strings.CutPrefix(n.ID, "__initial_"); initial {
				target := s.Transitions[s.States[parent].Start].To
				title := "開始: " + s.Label(target)
				fmt.Fprintf(&out, `<g class="initial-node" data-parent-state="%s" data-initial-state="%s" role="img" aria-label="%s" transform="translate(%.2f %.2f)"><title>%s</title><circle cx="%.2f" cy="%.2f" r="%.2f"/></g>`, esc(parent), esc(target), esc(title), n.X, n.Y, esc(title), n.Width/2, n.Height/2, math.Min(n.Width, n.Height)/2)
				continue
			}
			if s.IsDecision(n.ID) {
				label := s.Label(n.ID)
				fmt.Fprintf(&out, `<g class="decision" data-decision="%s" role="img" aria-label="%s" transform="translate(%.2f %.2f)"><title>%s</title><polygon points="%.2f,0 %.2f,%.2f %.2f,%.2f 0,%.2f"/>`, esc(n.ID), esc("ディシジョン: "+label), n.X, n.Y, esc(label), n.Width/2, n.Width, n.Height/2, n.Width/2, n.Height, n.Height/2)
				lines := wrapText(label, n.Width-24, 12)
				total := float64(len(lines)) * 16
				y := (n.Height-total)/2 + 12
				for _, line := range lines {
					fmt.Fprintf(&out, `<text x="%.2f" y="%.2f" text-anchor="middle">%s</text>`, n.Width/2, y, esc(line))
					y += 16
				}
				out.WriteString(`</g>`)
				continue
			}
			st, ok := s.States[n.ID]
			if !ok {
				return fmt.Errorf("unknown layout state")
			}
			composite := len(st.Children) > 0
			class := "node"
			if composite {
				class += " composite"
			}
			fmt.Fprintf(&out, `<g class="%s" data-state="%s" transform="translate(%.2f %.2f)"><title>%s</title><rect width="%.2f" height="%.2f" rx="9"/>`, class, esc(n.ID), n.X, n.Y, esc(n.ID), n.Width, n.Height)
			y := 25.0
			for _, line := range wrapText(st.Name, n.Width-30, 14) {
				fmt.Fprintf(&out, `<text x="15" y="%.2f">%s</text>`, y, esc(line))
				y += 18
			}
			if line := screenLine(st); line != "" {
				y += 3
				for _, l := range wrapText(line, n.Width-30, 11) {
					fmt.Fprintf(&out, `<text class="sub" x="15" y="%.2f">%s</text>`, y, esc(l))
					y += 13
				}
			}
			internal := internalTransitions(s, visible, n.ID)
			header := nodeHeader(st, n.Width)
			if composite || len(internal) > 0 {
				fmt.Fprintf(&out, `<line class="state-divider" x1="0" y1="%.2f" x2="%.2f" y2="%.2f"/>`, header, n.Width, header)
			}
			y = header + 17
			for _, id := range internal {
				t := s.Transitions[id]
				lines := internalLines(s, t, n.Width)
				class := "edge internal-transition"
				if !t.DefaultVisible {
					class += " transition-hidden"
				}
				fmt.Fprintf(&out, `<g class="%s" data-transition="%s" data-default-visible="%t" aria-hidden="%t" data-kind="internal"><title>%s</title><rect x="10" y="%.2f" width="%.2f" height="%.2f" rx="3"/>`, class, esc(id), t.DefaultVisible, !t.DefaultVisible, esc(transitionTitle(s, id, t)), y-13, n.Width-20, float64(len(lines))*14+6)
				for _, line := range lines {
					fmt.Fprintf(&out, `<text x="15" y="%.2f">%s</text>`, y, esc(line))
					y += 14
				}
				y += 8
				out.WriteString(`</g>`)
			}
			if composite && len(internal) > 0 {
				bottom := header + internalHeight(s, visible, n.ID, n.Width)
				fmt.Fprintf(&out, `<line class="state-divider" x1="0" y1="%.2f" x2="%.2f" y2="%.2f"/>`, bottom+6, n.Width, bottom+6)
			}
			if err := nodes(n.Children); err != nil {
				return err
			}
			out.WriteString(`</g>`)
		}
		return nil
	}
	if err := nodes(g.Children); err != nil {
		return "", err
	}
	for _, placed := range edges {
		ed := placed.connection
		if strings.HasPrefix(ed.ID, "__layout_layer_edge_") {
			continue
		}
		offset := placed.offset
		if ed.Container != "" {
			var ok bool
			offset, ok = offsets[ed.Container]
			if !ok {
				return "", fmt.Errorf("layout returned unknown edge container %s", ed.Container)
			}
		}
		if len(ed.Sections) == 0 {
			return "", fmt.Errorf("layout returned no route for %s", ed.ID)
		}
		t, isTransition := s.Transitions[ed.ID]
		if marker, start := strings.CutPrefix(ed.ID, "__start_edge_"); start {
			fmt.Fprintf(&out, `<g class="start-edge" data-start-state="%s"><title>%s</title>`, esc(starts[marker]), esc("開始: "+s.Label(starts[marker])))
		} else if isTransition && t.Kind == model.KindStart {
			fmt.Fprintf(&out, `<g class="initial-edge" data-transition="%s" data-parent-state="%s" data-initial-state="%s"><title>%s</title>`, esc(ed.ID), esc(t.From), esc(t.To), esc("開始: "+s.Label(t.To)))
		} else {
			if !isTransition {
				return "", fmt.Errorf("layout returned unknown edge")
			}
			class := "edge"
			if t.Kind == model.KindBranch {
				class += " decision-edge"
			}
			if !t.DefaultVisible {
				class += " transition-hidden"
			}
			fmt.Fprintf(&out, `<g class="%s" data-transition="%s" data-default-visible="%t" aria-hidden="%t"><title>%s</title>`, class, esc(ed.ID), t.DefaultVisible, !t.DefaultVisible, esc(transitionTitle(s, ed.ID, t)))
		}
		for _, sec := range ed.Sections {
			pts := append(append([]Point{sec.Start}, sec.Bends...), sec.End)
			out.WriteString(`<path marker-end="url(#arrow)" d="`)
			for i, p := range pts {
				if !finite(p.X) || !finite(p.Y) {
					return "", fmt.Errorf("non-finite edge route")
				}
				cmd := "L"
				if i == 0 {
					cmd = "M"
				}
				fmt.Fprintf(&out, "%s%.2f %.2f ", cmd, p.X+offset.X, p.Y+offset.Y)
			}
			out.WriteString(`"/>`)
		}
		for _, l := range ed.Labels {
			if !finite(l.X) || !finite(l.Y) || !finite(l.Width) || !finite(l.Height) || l.Width < 0 || l.Height < 0 {
				return "", fmt.Errorf("invalid label bounds")
			}
			fmt.Fprintf(&out, `<rect class="labelbox" x="%.2f" y="%.2f" width="%.2f" height="%.2f" rx="3"/>`, l.X+offset.X-3, l.Y+offset.Y-2, l.Width+6, l.Height+4)
			for i, line := range strings.Split(l.Text, "\n") {
				fmt.Fprintf(&out, `<text x="%.2f" y="%.2f">%s</text>`, l.X+offset.X, l.Y+offset.Y+12+float64(i)*14, esc(line))
			}
		}
		out.WriteString(`</g>`)
	}
	out.WriteString(`</svg>`)
	return out.String(), nil
}
func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) && math.Abs(f) < 1e8 }
func esc(s string) string   { return html.EscapeString(s) }

type diagram struct {
	ID  string
	SVG template.HTML
}
type view struct {
	Name      string
	Status    string
	States    int
	UseCases  int
	Scenarios int
	Error     string
	Diagrams  []diagram
	Data      template.JS
	CSS       template.CSS
	JS        template.JS
	CSP       string
}

func Build(s *model.Spec, r *model.Report, layout Layout, assets fs.FS) ([]byte, error) {
	v := view{Name: r.Name, Status: r.Status, States: r.ReachableStates, UseCases: len(r.UseCases), Scenarios: len(r.Scenarios)}
	if s == nil {
		s = model.NewSpec()
	}
	variants := map[string]map[string]string{"": {"[]": "system", "*": "system"}}
	if len(s.States) > 1 && len(s.Transitions) > 0 {
		rootStart := map[string]string{}
		if start, ok := s.Transitions[s.States[""].Start]; ok {
			rootStart["root"] = start.To
		}
		diagrams, index, err := buildDiagramVariants(s, r, "", DiagramTransitions(s, nil), rootStart, layout)
		if err != nil {
			return nil, err
		}
		v.Diagrams = append(v.Diagrams, diagrams...)
		variants[""] = index
		for _, id := range model.Keys(s.UseCases) {
			starts := map[string]string{}
			for i, start := range s.UseCases[id].Starts {
				starts[fmt.Sprintf("u%d", i)] = start
			}
			diagrams, index, err := buildDiagramVariants(s, r, id, DiagramTransitions(s, s.UseCases[id].Scope), starts, layout)
			if err != nil {
				return nil, fmt.Errorf("layout %s: %w", id, err)
			}
			v.Diagrams = append(v.Diagrams, diagrams...)
			variants[id] = index
		}
	}
	if len(v.Diagrams) == 0 {
		v.Diagrams = []diagram{{"system", `<p class="empty">有効な図を生成できません。診断を確認してください。</p>`}}
	}
	b, err := json.Marshal(map[string]any{"spec": s, "report": r, "diagramVariants": variants})
	if err != nil {
		return nil, err
	}
	v.Data = template.JS(b) // json.Marshal escapes HTML metacharacters and </script>.
	css, err := fs.ReadFile(assets, "viewer/style.css")
	if err != nil {
		return nil, err
	}
	js, err := fs.ReadFile(assets, "viewer/app.js")
	if err != nil {
		return nil, err
	}
	// HTML parsers normalize CRLF and CR before checking inline script hashes.
	js = bytes.ReplaceAll(js, []byte("\r\n"), []byte("\n"))
	js = bytes.ReplaceAll(js, []byte("\r"), []byte("\n"))
	v.CSS = template.CSS(css)
	v.JS = template.JS(js)
	hash := sha256.Sum256(js)
	v.CSP = "default-src 'none'; script-src 'sha256-" + base64.StdEncoding.EncodeToString(hash[:]) + "'; style-src 'unsafe-inline'; img-src data:; base-uri 'none'; object-src 'none'"
	t, err := template.New("report").Parse(page)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err = t.Execute(&out, v); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
func ErrorPage(message string) []byte {
	return []byte("<!doctype html><html lang=ja><meta charset=utf-8><title>ucspec build failed</title><h1>仕様書生成に失敗しました</h1><pre>" + esc(message) + "</pre><p>同じフォルダーの report.json を確認してください。</p></html>")
}

const page = `<!doctype html>
<html lang="ja"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="{{.CSP}}"><title>{{.Name}} | ucspec</title><style>{{.CSS}}</style></head>
<body><header class="topbar"><button id="toc-button" type="button" aria-controls="toc" aria-expanded="false">目次</button><strong class="product">{{.Name}}</strong><span id="crumbs" class="where"></span><span class="meta"><span class="status {{.Status}}">{{.Status}}</span><span>ユースケース {{.UseCases}}</span><span>シナリオ {{.Scenarios}}</span><span>末端状態 {{.States}}</span></span></header>
<div class="layout"><nav id="toc" aria-label="目次"><div class="toc-head"><span>目次</span><button id="toc-pin" type="button">左に固定</button></div><input id="filter" class="search" placeholder="シナリオを検索" aria-label="シナリオを検索"><div id="nav" class="nav-tree"></div></nav><main id="main"></main></div>
{{range .Diagrams}}<template id="diagram-{{.ID}}">{{.SVG}}</template>{{end}}
<script id="spec-data" type="application/json">{{.Data}}</script><script>{{.JS}}</script></body></html>`
