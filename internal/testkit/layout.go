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
	children, _ := g["children"].([]any)
	positions := map[string][2]float64{}
	for i, x := range children {
		n := x.(map[string]any)
		xx := float64(50 + (i%3)*340)
		yy := float64(60 + (i/3)*190)
		n["x"] = xx
		n["y"] = yy
		positions[n["id"].(string)] = [2]float64{xx, yy}
	}
	edges, _ := g["edges"].([]any)
	for i, x := range edges {
		e := x.(map[string]any)
		a := positions[e["sources"].([]any)[0].(string)]
		b := positions[e["targets"].([]any)[0].(string)]
		e["sections"] = []any{map[string]any{"startPoint": map[string]float64{"x": a[0] + 80, "y": a[1] + 60}, "endPoint": map[string]float64{"x": b[0] + 80, "y": b[1]}}}
		labels, _ := e["labels"].([]any)
		for _, l := range labels {
			label := l.(map[string]any)
			label["x"] = float64(40 + (i%3)*340)
			label["y"] = float64(150 + (i/3)*24)
		}
	}
	g["width"] = 1100
	g["height"] = float64(260 + (len(children)/3)*190 + len(edges)*24)
	return json.Marshal(g)
}
