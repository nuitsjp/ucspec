// Package report renders already-computed analysis. It never explores a model.
package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/nuitsjp/usecase-spec/internal/model"
	"html"
	"html/template"
	"io/fs"
	"math"
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
	ID     string  `json:"id"`
	X      float64 `json:"x,omitempty"`
	Y      float64 `json:"y,omitempty"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}
type Section struct {
	Start Point   `json:"startPoint"`
	End   Point   `json:"endPoint"`
	Bends []Point `json:"bendPoints,omitempty"`
}
type Connection struct {
	ID       string    `json:"id"`
	Sources  []string  `json:"sources"`
	Targets  []string  `json:"targets"`
	Labels   []Label   `json:"labels"`
	Sections []Section `json:"sections,omitempty"`
}
type Graph struct {
	ID       string         `json:"id"`
	Width    float64        `json:"width,omitempty"`
	Height   float64        `json:"height,omitempty"`
	Options  map[string]any `json:"layoutOptions,omitempty"`
	Children []Shape        `json:"children"`
	Edges    []Connection   `json:"edges"`
}

func graphFor(s *model.Spec, scope []string) Graph {
	g := Graph{ID: "root", Options: map[string]any{"elk.algorithm": "layered", "elk.direction": "RIGHT", "elk.randomSeed": 1, "elk.edgeRouting": "ORTHOGONAL", "elk.spacing.nodeNode": 45, "elk.layered.spacing.nodeNodeBetweenLayers": 100, "elk.padding": "[top=45,left=45,bottom=45,right=45]"}}
	ids := scope
	states := map[string]bool{}
	if ids == nil {
		ids = model.Keys(s.Transitions)
		for id := range s.States {
			states[id] = true
		}
	}
	for _, id := range ids {
		t := s.Transitions[id]
		states[t.From] = true
		states[t.To] = true
	}
	for _, id := range model.Keys(states) {
		st := s.States[id]
		w := math.Max(195, math.Min(350, measure(st.Label, 14)+40))
		height := 30 + float64(len(wrapText(st.Label, w-30, 14)))*18 + float64(len(wrapText(id+" · "+st.Screen, w-30, 11)))*13
		g.Children = append(g.Children, Shape{ID: id, Width: w, Height: math.Max(76, height)})
	}
	for _, id := range sorted(ids) {
		t := s.Transitions[id]
		event := s.Events[t.Event]
		source := map[string]string{"user": "ユーザー", "system": "システム", "timer": "タイマー"}[event.Source]
		lines := append(wrapText(id, 290, 11), wrapText(source+"："+event.Label, 290, 11)...)
		w := 0.0
		for _, line := range lines {
			w = math.Max(w, measure(line, 11))
		}
		g.Edges = append(g.Edges, Connection{ID: id, Sources: []string{t.From}, Targets: []string{t.To}, Labels: []Label{{Text: strings.Join(lines, "\n"), Width: w + 12, Height: float64(len(lines))*14 + 4}}})
	}
	return g
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
func draw(s *model.Spec, scope []string, layout Layout) (string, error) {
	g := graphFor(s, scope)
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
	var out strings.Builder
	fmt.Fprintf(&out, `<svg xmlns="http://www.w3.org/2000/svg" role="img" aria-label="状態遷移図" viewBox="0 0 %.2f %.2f"><defs><marker id="arrow" markerWidth="8" markerHeight="8" refX="7" refY="3" orient="auto"><path d="M0,0 L7,3 L0,6" fill="none" stroke="#728795"/></marker></defs>`, g.Width+20, g.Height+20)
	for _, ed := range g.Edges {
		t, ok := s.Transitions[ed.ID]
		if !ok {
			return "", fmt.Errorf("layout returned unknown edge")
		}
		if len(ed.Sections) == 0 {
			return "", fmt.Errorf("layout returned no route for %s", ed.ID)
		}
		fmt.Fprintf(&out, `<g class="edge" data-transition="%s"><title>%s</title>`, esc(ed.ID), esc(ed.ID+": "+s.Events[t.Event].Label+" / "+t.Action))
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
				fmt.Fprintf(&out, "%s%.2f %.2f ", cmd, p.X, p.Y)
			}
			out.WriteString(`"/>`)
		}
		for _, l := range ed.Labels {
			if !finite(l.X) || !finite(l.Y) || !finite(l.Width) || !finite(l.Height) || l.Width < 0 || l.Height < 0 {
				return "", fmt.Errorf("invalid label bounds")
			}
			fmt.Fprintf(&out, `<rect class="labelbox" x="%.2f" y="%.2f" width="%.2f" height="%.2f" rx="3"/>`, l.X-3, l.Y-2, l.Width+6, l.Height+4)
			for i, line := range strings.Split(l.Text, "\n") {
				fmt.Fprintf(&out, `<text x="%.2f" y="%.2f">%s</text>`, l.X, l.Y+12+float64(i)*14, esc(line))
			}
		}
		out.WriteString(`</g>`)
	}
	for _, n := range g.Children {
		st, ok := s.States[n.ID]
		if !ok {
			return "", fmt.Errorf("unknown layout state")
		}
		if !finite(n.X) || !finite(n.Y) || !finite(n.Width) || !finite(n.Height) || n.Width <= 0 || n.Height <= 0 {
			return "", fmt.Errorf("invalid node bounds")
		}
		fmt.Fprintf(&out, `<g class="node" data-state="%s"><title>%s</title><rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" rx="9"/>`, esc(n.ID), esc(s.Screens[st.Screen].Label), n.X, n.Y, n.Width, n.Height)
		y := n.Y + 25
		for _, line := range wrapText(st.Label, n.Width-30, 14) {
			fmt.Fprintf(&out, `<text x="%.2f" y="%.2f">%s</text>`, n.X+15, y, esc(line))
			y += 18
		}
		y += 3
		for _, line := range wrapText(n.ID+" · "+st.Screen, n.Width-30, 11) {
			fmt.Fprintf(&out, `<text class="sub" x="%.2f" y="%.2f">%s</text>`, n.X+15, y, esc(line))
			y += 13
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
	Version   string
	Hash      string
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
	v := view{Name: r.Name, Status: r.Status, Version: r.ToolVersion, Hash: r.InputHash, States: r.ReachableStates, UseCases: len(r.UseCases), Scenarios: len(r.Scenarios)}
	if s == nil {
		s = &model.Spec{Document: model.EmptyDocument()}
	}
	if len(s.States) > 0 && len(s.Transitions) > 0 {
		svg, err := draw(s, nil, layout)
		if err != nil {
			return nil, err
		}
		v.Diagrams = append(v.Diagrams, diagram{"system", template.HTML(svg)})
		for _, id := range model.Keys(s.UseCases) {
			svg, err := draw(s, s.UseCases[id].Scope, layout)
			if err != nil {
				return nil, fmt.Errorf("layout %s: %w", id, err)
			}
			v.Diagrams = append(v.Diagrams, diagram{"uc-" + id, template.HTML(svg)})
		}
	}
	if len(v.Diagrams) == 0 {
		v.Diagrams = []diagram{{"system", `<p class="empty">有効な図を生成できません。診断を確認してください。</p>`}}
	}
	b, err := json.Marshal(map[string]any{"spec": s, "report": r})
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
	return []byte("<!doctype html><html lang=ja><meta charset=utf-8><title>UCSPEC build failed</title><h1>仕様書生成に失敗しました</h1><pre>" + esc(message) + "</pre><p>同じフォルダーの report.json を確認してください。</p></html>")
}

const page = `<!doctype html>
<html lang="ja"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="{{.CSP}}"><title>{{.Name}} | UCSPEC</title><style>{{.CSS}}</style></head>
<body><header><div class="eyebrow">UCSPEC / EXECUTABLE USE-CASE SPECIFICATION</div><h1>{{.Name}}</h1><p>目的から仕様へ。共有された振る舞いを、シナリオと検証で確認する。</p></header>
<div class="layout"><aside><strong>ユースケース</strong><div id="nav"></div><hr><label for="filter">シナリオを検索</label><input id="filter" class="search" placeholder="シナリオ名"><div id="scenario-list"></div><hr><a href="#coverage">網羅率・未網羅項目</a><br><a href="#checks">モデル検査</a><br><a href="#diagnostics">診断</a></aside>
<main><div class="summary"><div class="stat"><small>検証結果</small><strong class="status {{.Status}}">{{.Status}}</strong></div><div class="stat"><small>到達状態（画面＋データ）</small><strong>{{.States}}</strong></div><div class="stat"><small>ユースケース</small><strong>{{.UseCases}}</strong></div><div class="stat"><small>登録シナリオ・回帰ケース</small><strong>{{.Scenarios}}</strong></div></div>
<div class="notice">ここでの網羅率は<strong>シナリオ定義</strong>の網羅性です。実装のテスト実行結果ではありません。検証済みは、定義した有限モデルと検査条件の範囲に限ります。</div>
<section class="card" id="uc-summary"></section><section class="card"><h2>画面・状態遷移</h2><div class="toolbar"><select id="view-select" aria-label="表示ユースケース"></select><button id="zoom-out" aria-label="縮小">−</button><span id="zoom-label"></span><button id="zoom-in" aria-label="拡大">＋</button><button id="zoom-fit">全体を表示</button><span class="mutedtext small">状態・矢印をクリックすると詳細を表示</span></div><div id="graph" class="diagram"></div></section>
<section class="card" id="detail"><h2>詳細</h2><p class="mutedtext">シナリオ、遷移、未網羅項目、反例を選択してください。</p></section><section class="card" id="coverage"></section>
<section class="card" id="checks"><h2>モデル検査・独立した期待結果</h2></section><section class="card" id="diagnostics"><h2>診断</h2></section><section class="card" id="impact"></section>
<section class="card"><h2>再現性</h2><p>ツール: {{.Version}}</p><p>入力 SHA-256: <code>{{.Hash}}</code></p><button id="download-json">検査結果JSONを書き出す</button></section></main></div>
<footer>モデルの期待結果は実装の保証ではありません。未網羅候補は自動承認されません。ネットワーク不要の自己完結型HTML。</footer>
{{range .Diagrams}}<template id="diagram-{{.ID}}">{{.SVG}}</template>{{end}}
<script id="spec-data" type="application/json">{{.Data}}</script><script>{{.JS}}</script></body></html>`
