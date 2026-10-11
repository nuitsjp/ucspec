package testkit

import "encoding/json"

// FixtureLayout is deliberately NOT an ELK substitute. Tests use fixed simple
// coordinates to verify the renderer contract without downloading dependencies.
// Production main ALWAYS wires adapters.LayoutJSON (elk-go).
func FixtureLayout(b []byte) ([]byte, error) {
	var g map[string]any
	if e := json.Unmarshal(b, &g); e != nil {
		return nil, e
	}
	var size func(map[string]any)
	size = func(parent map[string]any) {
		children, _ := parent["children"].([]any)
		if len(children) == 0 {
			return
		}
		x, maxHeight := 50.0, 0.0
		for _, child := range children {
			n := child.(map[string]any)
			size(n)
			n["x"], n["y"] = x, 100.0
			x += n["width"].(float64) + 100
			if height := n["height"].(float64); height > maxHeight {
				maxHeight = height
			}
		}
		parent["width"], parent["height"] = x, maxHeight+150
	}
	size(g)
	positions := map[string][2]float64{"root": {}}
	var locate func(map[string]any, [2]float64)
	locate = func(parent map[string]any, point [2]float64) {
		children, _ := parent["children"].([]any)
		for _, child := range children {
			n := child.(map[string]any)
			p := [2]float64{point[0] + n["x"].(float64), point[1] + n["y"].(float64)}
			positions[n["id"].(string)] = p
			locate(n, p)
		}
	}
	locate(g, [2]float64{})
	var route func(map[string]any)
	route = func(parent map[string]any) {
		origin := positions[parent["id"].(string)]
		edges, _ := parent["edges"].([]any)
		for i, edge := range edges {
			e := edge.(map[string]any)
			a := positions[e["sources"].([]any)[0].(string)]
			b := positions[e["targets"].([]any)[0].(string)]
			e["container"] = parent["id"]
			e["sections"] = []any{map[string]any{"startPoint": map[string]float64{"x": a[0] - origin[0] + 10, "y": a[1] - origin[1] + 20}, "endPoint": map[string]float64{"x": b[0] - origin[0] + 10, "y": b[1] - origin[1]}}}
			labels, _ := e["labels"].([]any)
			for _, label := range labels {
				l := label.(map[string]any)
				l["x"], l["y"] = a[0]-origin[0], a[1]-origin[1]+float64(30+i*24)
			}
		}
		children, _ := parent["children"].([]any)
		for _, child := range children {
			route(child.(map[string]any))
		}
	}
	route(g)
	return json.Marshal(g)
}
