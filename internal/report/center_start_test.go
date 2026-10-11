package report

import (
	"math"
	"reflect"
	"testing"
)

func TestCenteredStartKeepsELKNodesAndArrowTarget(t *testing.T) {
	g := Graph{Width: 600, Height: 400,
		Children: []Shape{
			{ID: "__start_search", X: 400, Y: 45, Width: 20, Height: 20},
			{ID: "search", X: 360, Y: 165, Width: 195, Height: 76},
		},
		Edges: []Connection{
			{ID: "__start_edge_search", Sections: []Section{{Start: Point{410, 65}, End: Point{375, 165}, Bends: []Point{{410, 125}, {375, 125}}}}},
			{ID: "business", Sections: []Section{{Start: Point{375, 241}, End: Point{500, 350}, Bends: []Point{{375, 300}, {500, 300}}}}},
		},
	}
	state, business := g.Children[1], g.Edges[1]
	target := g.Edges[0].Sections[0].End
	centerStartMarkers(&g)
	marker := g.Children[0]
	if marker.X+marker.Width/2 != (g.Width+20)/2 || marker.Y != 45 {
		t.Fatal("start is not at the top center")
	}
	if !reflect.DeepEqual(state, g.Children[1]) || !reflect.DeepEqual(business, g.Edges[1]) {
		t.Fatal("centering moved a semantic node or business route")
	}
	section := g.Edges[0].Sections[0]
	if section.Start != (Point{marker.X + marker.Width/2, marker.Y + marker.Height}) || section.End != target {
		t.Fatal("start arrow lost its marker or target")
	}
	points := append(append([]Point{section.Start}, section.Bends...), section.End)
	for i := 1; i < len(points); i++ {
		if points[i-1].X != points[i].X && points[i-1].Y != points[i].Y {
			t.Fatal("start arrow is not orthogonal")
		}
	}
	for _, bend := range section.Bends[:2] {
		if bend.Y <= marker.Y+marker.Height || bend.Y >= state.Y {
			t.Fatal("start arrow crosses the top-level node row")
		}
	}
}

func TestMultipleStartMarkersFormCenteredRow(t *testing.T) {
	g := Graph{Width: 600, Height: 400, Children: []Shape{
		{ID: "__start_a", X: 40, Y: 45, Width: 20, Height: 20},
		{ID: "__start_b", X: 400, Y: 45, Width: 20, Height: 20},
		{ID: "a", X: 40, Y: 165, Width: 195, Height: 76},
	}}
	centerStartMarkers(&g)
	a, b := g.Children[0], g.Children[1]
	if a.X+a.Width >= b.X || a.Y != b.Y || math.Abs((a.X+b.X+b.Width)/2-(g.Width+20)/2) > .001 {
		t.Fatal("multiple start markers overlap or are not centered together")
	}
}
