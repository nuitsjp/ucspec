package adapters

import (
	"encoding/xml"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"testing"

	"github.com/nuitsjp/ucspec/internal/assets"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/report"
	"github.com/nuitsjp/ucspec/internal/testkit"
)

func assertActualCenteredStarts(t *testing.T, page []byte) {
	t.Helper()
	svgs := regexp.MustCompile(`(?s)<svg\b[^>]*>.*?</svg>`).FindAll(page, -1)
	if len(svgs) == 0 {
		t.Fatal("no diagrams generated")
	}
	for _, svg := range svgs {
		var diagram struct {
			ViewBox string `xml:"viewBox,attr"`
			Groups  []struct {
				Class     string `xml:"class,attr"`
				State     string `xml:"data-start-state,attr"`
				Transform string `xml:"transform,attr"`
				Circle    struct {
					X      float64 `xml:"cx,attr"`
					Y      float64 `xml:"cy,attr"`
					Radius float64 `xml:"r,attr"`
				} `xml:"circle"`
				Paths []struct {
					D string `xml:"d,attr"`
				} `xml:"path"`
			} `xml:"g"`
		}
		if err := xml.Unmarshal(svg, &diagram); err != nil {
			t.Fatal(err)
		}
		var width, height float64
		if _, err := fmt.Sscanf(diagram.ViewBox, "0 0 %f %f", &width, &height); err != nil {
			t.Fatal(err)
		}
		markers := map[string]report.Point{}
		radii := map[string]float64{}
		left, right := math.Inf(1), math.Inf(-1)
		for _, group := range diagram.Groups {
			if group.Class != "start-node" {
				continue
			}
			var x, y float64
			if _, err := fmt.Sscanf(group.Transform, "translate(%f %f)", &x, &y); err != nil {
				t.Fatal(err)
			}
			center := report.Point{X: x + group.Circle.X, Y: y + group.Circle.Y}
			markers[group.State], radii[group.State] = center, group.Circle.Radius
			left = math.Min(left, center.X-group.Circle.Radius)
			right = math.Max(right, center.X+group.Circle.Radius)
		}
		if len(markers) == 0 || math.Abs((left+right)/2-width/2) > .02 {
			t.Fatal("start row is not centered in its SVG")
		}
		for _, group := range diagram.Groups {
			if group.Class != "start-edge" {
				continue
			}
			for _, path := range group.Paths {
				coordinates := regexp.MustCompile(`(?:M|L)([-0-9.]+) ([-0-9.]+)`).FindAllStringSubmatch(path.D, -1)
				if len(coordinates) < 2 {
					t.Fatal("start arrow has no route")
				}
				var previous report.Point
				for i, coordinate := range coordinates {
					x, _ := strconv.ParseFloat(coordinate[1], 64)
					y, _ := strconv.ParseFloat(coordinate[2], 64)
					point := report.Point{X: x, Y: y}
					if i == 0 {
						center := markers[group.State]
						if math.Abs(math.Hypot(point.X-center.X, point.Y-center.Y)-radii[group.State]) > .03 {
							t.Fatal("start arrow is detached from its centered marker")
						}
					} else if math.Abs(point.X-previous.X) > .02 && math.Abs(point.Y-previous.Y) > .02 {
						t.Fatal("centered start arrow has a diagonal segment")
					}
					previous = point
				}
			}
		}
	}
}

func TestActualELKCenteredStartsInAllSampleDiagrams(t *testing.T) {
	page, err := report.Build(testkit.Sample(t), &model.Report{}, LayoutJSON, assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	assertActualCenteredStarts(t, page)
}
