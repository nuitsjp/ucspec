package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	"github.com/nuitsjp/ucspec/internal/model"
)

// Layout every selectable transition set at generation time. The viewer only
// switches SVGs, keeping the report self-contained and independent of ELK.
func buildDiagramVariants(s *model.Spec, r *model.Report, viewID string, scope []string, starts map[string]string, layout Layout) ([]diagram, map[string]string, error) {
	base := "system"
	if viewID != "" {
		base = "uc-" + viewID
	}
	all := scope
	normal, hidden := []string{}, []string{}
	allowed := map[string]bool{}
	for _, id := range sorted(all) {
		allowed[id] = true
		if s.Transitions[id].DefaultVisible {
			normal = append(normal, id)
		} else {
			hidden = append(hidden, id)
		}
	}
	keyFor := func(path []string) (string, []string) {
		ids := []string{}
		for _, id := range sorted(path) {
			if allowed[id] && !s.Transitions[id].DefaultVisible {
				ids = append(ids, id)
			}
		}
		return variantKey(ids), ids
	}
	index := map[string]string{"[]": base, "*": base + "-all"}
	if key, _ := keyFor(hidden); key != "[]" {
		index[key] = base + "-all"
	}
	selections := map[string][]string{}
	addPath := func(path []string) {
		key, ids := keyFor(path)
		if _, exists := index[key]; !exists {
			selections[key] = ids
		}
	}
	addWitness := func(w *model.Witness) {
		if w == nil {
			return
		}
		addPath(w.Transitions)
	}
	// A hidden edge clicked in the all-transitions view remains selectable when
	// the viewer returns to the normal visibility mode.
	for _, id := range hidden {
		addPath([]string{id})
	}
	for _, scenario := range r.Scenarios {
		if scenario.UseCase == viewID {
			addWitness(&scenario.Witness)
		}
	}
	for _, usecase := range r.UseCases {
		if usecase.ID != viewID {
			continue
		}
		for _, coverage := range usecase.Coverage {
			for _, missing := range coverage.Missing {
				addPath(missing.Path)
				addWitness(missing.Witness)
			}
		}
	}
	if viewID == "" {
		for _, check := range r.Checks {
			addWitness(check.Witness)
		}
		for _, diagnostic := range r.Diagnostics {
			addWitness(diagnostic.Witness)
		}
	}
	diagrams := []diagram{}
	appendDiagram := func(id string, visible []string) error {
		svg, err := draw(s, scope, visible, starts, layout)
		if err != nil {
			return fmt.Errorf("diagram %s: %w", id, err)
		}
		diagrams = append(diagrams, diagram{id, template.HTML(svg)})
		return nil
	}
	if err := appendDiagram(base, normal); err != nil {
		return nil, nil, err
	}
	if len(hidden) == 0 {
		// Both modes have the same transition set; share the layout and SVG.
		index["*"] = base
	} else if err := appendDiagram(base+"-all", all); err != nil {
		return nil, nil, err
	}
	for i, key := range model.Keys(selections) {
		id := fmt.Sprintf("%s-selected-%d", base, i+1)
		visible := append(append([]string{}, normal...), selections[key]...)
		if err := appendDiagram(id, visible); err != nil {
			return nil, nil, err
		}
		index[key] = id
	}
	return diagrams, index, nil
}

// variantKey matches JSON.stringify in the viewer: no HTML escaping, so that
// transition IDs containing ">" produce the same key in Go and JavaScript.
func variantKey(ids []string) string {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(ids)
	return strings.TrimSuffix(b.String(), "\n")
}
