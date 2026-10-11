package adapters

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/assets"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/report"
)

func TestActualELKNestedStates(t *testing.T) {
	s := &model.Spec{
		Project: model.Project{Initial: []model.Snapshot{{State: "view"}}},
		Document: model.Document{
			Screens: map[string]model.Screen{"main": {Label: "Main"}},
			States: map[string]model.State{
				"view":       {Label: "顧客閲覧", Screen: "main", Initial: "viewing"},
				"viewing":    {Label: "通常表示", Screen: "main", Parent: "view"},
				"confirming": {Label: "削除確認", Screen: "main", Parent: "view"},
				"deleting":   {Label: "削除中", Screen: "main", Parent: "view"},
				"end":        {Label: "完了", Screen: "main"},
			},
			Events: map[string]model.Event{"go": {Label: "Go", Source: "user"}},
			Transitions: map[string]model.Transition{
				"open":    {From: "end", To: "view", Event: "go", DefaultVisible: true},
				"confirm": {From: "viewing", To: "confirming", Event: "go", DefaultVisible: true},
				"delete":  {From: "confirming", To: "deleting", Event: "go", DefaultVisible: true},
				"done":    {From: "deleting", To: "end", Event: "go", DefaultVisible: true},
				"leave":   {From: "view", To: "end", Event: "go", DefaultVisible: true},
			},
			UseCases: map[string]model.UseCase{"delete": {Name: "Delete", Starts: []model.Snapshot{{State: "deleting"}}, Scope: []string{"done"}}},
		},
	}
	var laidOut []report.Graph
	layout := func(b []byte) ([]byte, error) {
		b, err := LayoutJSON(b)
		if err != nil {
			return nil, err
		}
		var g report.Graph
		if err := json.Unmarshal(b, &g); err != nil {
			return nil, err
		}
		assertNetworkSimplex(t, g)
		laidOut = append(laidOut, g)
		return b, nil
	}
	b, err := report.Build(s, &model.Report{}, layout, assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	if len(laidOut) != 2 {
		t.Fatalf("got %d diagrams", len(laidOut))
	}
	point := regexp.MustCompile(`[ML](-?\d+\.\d+) (-?\d+\.\d+)`)
	for i, id := range []string{"diagram-system", "diagram-uc-delete"} {
		text := string(b)
		open := `<template id="` + id + `">`
		start := strings.Index(text, open) + len(open)
		if start < len(open) {
			t.Fatalf("template %s missing", id)
		}
		end := strings.Index(text[start:], "</template>")
		if end < 0 {
			t.Fatalf("template %s unclosed", id)
		}
		svg := text[start : start+end]
		parents := svgHierarchy(t, svg)
		if parents["deleting"] != "view" || parents["viewing"] != "view" {
			t.Fatalf("ELK lost hierarchy: %#v", parents)
		}
		for _, p := range point.FindAllStringSubmatch(svg, -1) {
			x, _ := strconv.ParseFloat(p[1], 64)
			y, _ := strconv.ParseFloat(p[2], 64)
			if x < 0 || y < 0 || x > laidOut[i].Width+.01 || y > laidOut[i].Height+.01 {
				t.Fatalf("ELK edge point %.2f %.2f outside diagram %.2f %.2f", x, y, laidOut[i].Width, laidOut[i].Height)
			}
		}
	}
}

func assertNetworkSimplex(t *testing.T, g report.Graph) {
	t.Helper()
	if g.Options["elk.layered.nodePlacement.strategy"] != "NETWORK_SIMPLEX" {
		t.Fatalf("root placement strategy: %#v", g.Options)
	}
	var visit func([]report.Shape)
	visit = func(nodes []report.Shape) {
		for _, node := range nodes {
			if len(node.Children) > 0 && node.Options["elk.layered.nodePlacement.strategy"] != "NETWORK_SIMPLEX" {
				t.Fatalf("composite %s placement strategy: %#v", node.ID, node.Options)
			}
			visit(node.Children)
		}
	}
	visit(g.Children)
}

func svgHierarchy(t *testing.T, svg string) map[string]string {
	t.Helper()
	parents := map[string]string{}
	stack := []string{}
	d := xml.NewDecoder(strings.NewReader(svg))
	for {
		token, err := d.Token()
		if err == io.EOF {
			return parents
		}
		if err != nil {
			t.Fatalf("invalid SVG: %v", err)
		}
		switch token := token.(type) {
		case xml.StartElement:
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			for _, a := range token.Attr {
				if a.Name.Local == "data-state" {
					parents[a.Value] = parent
					parent = a.Value
				}
			}
			stack = append(stack, parent)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
}
