// Package compiler resolves source files into a single, deterministically ordered model.
package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/nuitsjp/usecase-spec/internal/engine"
	"github.com/nuitsjp/usecase-spec/internal/model"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

type Codec struct {
	Decode   func([]byte, string) ([]byte, error)
	Validate func(string, any) error
}

func JSONCodec() Codec {
	return Codec{Decode: func(b []byte, _ string) ([]byte, error) { return b, nil }}
}
func decode(c Codec, b []byte, name, kind string, out any) error {
	if len(b) > 8<<20 {
		return fmt.Errorf("source exceeds 8 MiB")
	}
	var err error
	if c.Decode != nil {
		b, err = c.Decode(b, filepath.Ext(name))
		if err != nil {
			return err
		}
	}
	if err = rejectDuplicateKeys(b); err != nil {
		return err
	}
	var doc any
	if err = json.Unmarshal(b, &doc); err != nil {
		return err
	}
	if c.Validate != nil {
		if err = c.Validate(kind, doc); err != nil {
			return err
		}
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(out); err != nil {
		return err
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected exactly one document")
	}
	return nil
}
func rejectDuplicateKeys(b []byte) error {
	d := json.NewDecoder(bytes.NewReader(b))
	var value func() error
	value = func() error {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		del, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch del {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return e
				}
				key, ok := k.(string)
				if !ok {
					return fmt.Errorf("non-string key")
				}
				if seen[key] {
					return fmt.Errorf("duplicate JSON key %q", key)
				}
				seen[key] = true
				if e = value(); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if err = value(); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("invalid JSON delimiter")
		}
		_, err = d.Token()
		return err
	}
	if err := value(); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON content")
	}
	return nil
}
func ReadProject(path string, c Codec) (model.Project, error) {
	var p model.Project
	b, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	err = decode(c, b, path, "project", &p)
	if err != nil {
		return p, fmt.Errorf("%s: %w", path, err)
	}
	p.Limits.Defaults()
	return p, nil
}
func Load(path string, c Codec) (*model.Spec, error) {
	p, err := ReadProject(path, c)
	if err != nil {
		return nil, err
	}
	s := &model.Spec{Project: p, Document: model.EmptyDocument(), Sources: map[string]model.Source{}}
	root, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	all := map[string]bool{}
	for _, pattern := range p.Files {
		if filepath.IsAbs(pattern) || strings.Contains(filepath.ToSlash(pattern), "../") || pattern == ".." {
			return nil, fmt.Errorf("source pattern must stay in spec directory: %s", pattern)
		}
		matches, er := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		if er != nil {
			return nil, er
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("source pattern matched no files: %s", pattern)
		}
		for _, m := range matches {
			if all[m] {
				return nil, fmt.Errorf("source included more than once: %s", m)
			}
			all[m] = true
		}
	}
	for _, name := range model.Keys(all) {
		real, er := filepath.EvalSymlinks(name)
		if er != nil {
			return nil, er
		}
		rel, er := filepath.Rel(realRoot, real)
		if er != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("source escapes spec directory: %s", name)
		}
		b, er := os.ReadFile(name)
		if er != nil {
			return nil, er
		}
		var doc model.Document
		if er = decode(c, b, name, "document", &doc); er != nil {
			return nil, fmt.Errorf("%s: %w", name, er)
		}
		if doc.SchemaVersion != "1" {
			return nil, fmt.Errorf("%s: unsupported schemaVersion %q", name, doc.SchemaVersion)
		}
		display, _ := filepath.Rel(root, name)
		display = filepath.ToSlash(display)
		if er = merge(s, doc, display); er != nil {
			return nil, er
		}
	}
	s.Hash = model.Hash(struct {
		Project  model.Project
		Document model.Document
	}{p, s.Document})
	return s, nil
}
func merge(s *model.Spec, doc model.Document, file string) error {
	dst := reflect.ValueOf(&s.Document).Elem()
	src := reflect.ValueOf(doc)
	typ := dst.Type()
	for i := 0; i < dst.NumField(); i++ {
		d := dst.Field(i)
		if d.Kind() != reflect.Map {
			continue
		}
		v := src.Field(i)
		section := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		keys := v.MapKeys()
		sort.Slice(keys, func(a, b int) bool { return keys[a].String() < keys[b].String() })
		for _, k := range keys {
			val := v.MapIndex(k)
			id := section + "/" + k.String()
			if d.MapIndex(k).IsValid() {
				return fmt.Errorf("duplicate %s in %s (first: %s)", id, file, s.Sources[id].File)
			}
			d.SetMapIndex(k, val)
			s.Sources[id] = model.Source{File: file, Pointer: "/" + section + "/" + escape(k.String())}
		}
	}
	return nil
}
func escape(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }

var idRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)

func Validate(s *model.Spec) []model.Diagnostic {
	d := []model.Diagnostic{}
	add := func(id, msg string) {
		d = append(d, model.Diagnostic{Code: "structure", Severity: "error", Subject: id, Message: msg, Source: s.Sources[id]})
	}
	p := s.Project
	if p.SchemaVersion != "1" {
		add("project", "unsupported schemaVersion")
	}
	if p.Name == "" {
		add("project", "name is required")
	}
	if len(p.Files) == 0 {
		add("project", "files must not be empty")
	}
	if len(p.Initial) == 0 {
		add("project", "at least one initial configuration is required")
	}
	if p.Output == "" {
		add("project", "output is required")
	}
	l := p.Limits
	if l.States < 1 || l.Steps < 1 || l.Paths < 1 || l.Combinations < 1 || l.States > 1000000 || l.Steps > 256 || l.Paths > 2000000 || l.Combinations > 100000 {
		add("project", "limits outside safe supported ranges")
	}
	for _, k := range model.Keys(p.Coverage) {
		v := p.Coverage[k]
		if !model.Contains(model.CoverageKinds, k) {
			add("project", "unknown coverage kind "+k)
		}
		if v < 0 || v > 100 {
			add("project", "coverage threshold must be 0..100")
		}
	}
	for _, id := range model.Keys(s.Sources) {
		part := strings.SplitN(id, "/", 2)[1]
		if !idRE.MatchString(part) {
			add(id, "ID must match "+idRE.String())
		}
	}
	for _, id := range model.Keys(s.Variables) {
		v := s.Variables[id]
		subject := "variables/" + id
		if len(v.Domain) == 0 {
			add(subject, "domain must not be empty")
		}
		if !model.InDomain(v.Domain, v.Initial) {
			add(subject, "initial value outside domain")
		}
		seen := map[string]bool{}
		for _, x := range v.Domain {
			switch x.(type) {
			case string, bool, float64, int:
			default:
				add(subject, "domain values must be scalar strings, booleans or numbers")
			}
			key := model.JSON(x)
			if seen[key] {
				add(subject, "duplicate domain value")
			}
			seen[key] = true
		}
	}
	for _, st := range p.Initial {
		if _, err := s.Normalize(st); err != nil {
			add("project", err.Error())
		}
	}
	if len(s.States) == 0 || len(s.UseCases) == 0 {
		add("project", "at least one state and one use case are required")
	}
	for _, id := range model.Keys(s.Screens) {
		if s.Screens[id].Label == "" {
			add("screens/"+id, "label required")
		}
	}
	for _, id := range model.Keys(s.States) {
		st := s.States[id]
		sub := "states/" + id
		if _, ok := s.Screens[st.Screen]; !ok {
			add(sub, "unknown screen "+st.Screen)
		}
		if st.Label == "" {
			add(sub, "label required")
		}
		seen := map[string]bool{}
		for _, ev := range st.Events {
			if _, ok := s.Events[ev]; !ok {
				add(sub, "unknown event "+ev)
			}
			if seen[ev] {
				add(sub, "duplicate event "+ev)
			}
			seen[ev] = true
		}
		if st.Terminal && len(st.Events) > 0 {
			add(sub, "terminal states must not accept events")
		}
	}
	validateExpr := func(id string, x any, params map[string][]any) {
		if err := engine.ValidateExpr(s, x, params, nil); err != nil {
			add(id, err.Error())
		}
	}
	unionParams := map[string][]any{}
	for _, id := range model.Keys(s.Events) {
		ev := s.Events[id]
		sub := "events/" + id
		if !model.Contains([]string{"user", "system", "timer"}, ev.Source) {
			add(sub, "source must be user/system/timer")
		}
		if ev.Label == "" {
			add(sub, "label required")
		}
		for _, k := range model.Keys(ev.Parameters) {
			vals := ev.Parameters[k]
			unionParams[k] = vals
			if !idRE.MatchString(k) || len(vals) == 0 {
				add(sub, "invalid parameter name or empty domain")
			}
			seen := map[string]bool{}
			for _, v := range vals {
				switch v.(type) {
				case bool, string, float64, int:
				default:
					add(sub, "event parameter domains must contain scalars")
				}
				key := model.JSON(v)
				if seen[key] {
					add(sub, "duplicate parameter domain value")
				}
				seen[key] = true
			}
		}
		if ev.When != nil {
			validateExpr(sub, ev.When, ev.Parameters)
		}
	}
	for _, id := range model.Keys(s.Conditions) {
		validateExpr("conditions/"+id, s.Conditions[id], unionParams)
		if !atomicShape(s.Conditions[id]) {
			add("conditions/"+id, "named atomic predicates must not embed and/or/condition; compose them at the guard")
		}
	}
	owned := map[string]bool{}
	for _, id := range model.Keys(s.Transitions) {
		t := s.Transitions[id]
		sub := "transitions/" + id
		st, ok := s.States[t.From]
		if !ok {
			add(sub, "unknown from state "+t.From)
		}
		if _, ok = s.States[t.To]; !ok {
			add(sub, "unknown to state "+t.To)
		}
		ev, ok := s.Events[t.Event]
		if !ok {
			add(sub, "unknown event "+t.Event)
		}
		if !model.Contains(st.Events, t.Event) {
			add(sub, "event not declared on source state")
		}
		if t.Action == "" {
			add(sub, "action description required")
		}
		if t.Guard != nil {
			validateExpr(sub, t.Guard, ev.Parameters)
			if !guardShape(t.Guard) {
				add(sub, "guards use named conditions with and/or/not; declare atomic predicates in conditions")
			}
		}
		for _, k := range model.Keys(t.Set) {
			x := t.Set[k]
			if _, ok := s.Variables[k]; !ok {
				add(sub, "assignment to unknown variable "+k)
			}
			validateExpr(sub, x, ev.Parameters)
		}
	}
	checkScenario := func(sub string, sc model.Scenario) {
		if sc.Name == "" {
			add(sub, "scenario name required")
		}
		if _, err := s.Normalize(sc.Start); err != nil {
			add(sub, err.Error())
		}
		validateExpr(sub, sc.Expect, nil)
		for _, step := range sc.Steps {
			if _, ok := s.Transitions[step.Transition]; !ok {
				add(sub, "unknown transition "+step.Transition)
			}
			if step.Expect != nil {
				validateExpr(sub, step.Expect, nil)
			}
		}
	}
	for _, id := range model.Keys(s.UseCases) {
		u := s.UseCases[id]
		sub := "usecases/" + id
		if u.Name == "" || u.Goal == "" || u.Actor == "" {
			add(sub, "name, goal and actor are required")
		}
		if len(u.Starts) == 0 || len(u.Scope) == 0 {
			add(sub, "starts and scope are required")
		}
		for _, st := range u.Starts {
			if n, err := s.Normalize(st); err != nil {
				add(sub, err.Error())
			} else if b, err := engine.Predicate(s, n, u.End); err != nil {
				add(sub, err.Error())
			} else if b {
				add(sub, "use-case start must not already satisfy end")
			}
		}
		validateExpr(sub, u.End, nil)
		seen := map[string]bool{}
		for _, tr := range u.Scope {
			if _, ok := s.Transitions[tr]; !ok {
				add(sub, "unknown transition "+tr)
			}
			if seen[tr] {
				add(sub, "duplicate scope transition "+tr)
			}
			seen[tr] = true
			owned[tr] = true
		}
		for _, sid := range model.Keys(u.Scenarios) {
			if !idRE.MatchString(sid) {
				add(sub, "invalid scenario ID "+sid)
			}
			checkScenario(sub, u.Scenarios[sid])
		}
	}
	for _, id := range model.Keys(s.Transitions) {
		if !owned[id] && s.Transitions[id].UnownedReason == "" {
			d = append(d, model.Diagnostic{Code: "unowned-transition", Severity: "warning", Subject: "transitions/" + id, Message: "transition has no use-case owner and no explicit reason", Source: s.Sources["transitions/"+id]})
		}
	}
	for _, id := range model.Keys(s.Properties) {
		v := s.Properties[id]
		sub := "properties/" + id
		if !model.Contains([]string{"invariant", "reachable", "recoverable"}, v.Kind) {
			add(sub, "unknown property kind")
		}
		validateExpr(sub, v.Assert, nil)
	}
	for _, id := range model.Keys(s.Regressions) {
		checkScenario("regressions/"+id, s.Regressions[id])
	}
	for _, id := range model.Keys(s.Decisions) {
		v := s.Decisions[id]
		sub := "decisions/" + id
		if _, ok := s.States[v.State]; !ok {
			add(sub, "unknown state")
		}
		if _, ok := s.Events[v.Event]; !ok {
			add(sub, "unknown event")
		}
		if len(v.Cases) == 0 {
			add(sub, "decision cases required")
		}
		for _, c := range v.Cases {
			if _, err := s.Normalize(model.Snapshot{State: v.State, Values: c.Values}); err != nil {
				add(sub, err.Error())
			}
			for _, tr := range c.Enabled {
				t, ok := s.Transitions[tr]
				if !ok || t.From != v.State || t.Event != v.Event {
					add(sub, "expected transition does not match table state/event: "+tr)
				}
			}
		}
	}
	sort.SliceStable(d, func(i, j int) bool { return d[i].Subject+d[i].Message < d[j].Subject+d[j].Message })
	return d
}
func guardShape(x any) bool {
	if _, ok := x.(bool); ok {
		return true
	}
	m, ok := x.(map[string]any)
	if !ok || len(m) != 1 {
		return false
	}
	for op, v := range m {
		switch op {
		case "condition":
			_, ok := v.(string)
			return ok
		case "not":
			return guardShape(v)
		case "and", "or":
			a, ok := v.([]any)
			if !ok {
				return false
			}
			for _, z := range a {
				if !guardShape(z) {
					return false
				}
			}
			return len(a) > 0
		}
	}
	return false
}

// Treat named predicates as the independently observable logical atoms. Do not
// permit hidden compound expressions inside a comparison or a negation.
func atomicShape(x any) bool {
	switch v := x.(type) {
	case map[string]any:
		for op, arg := range v {
			if op == "and" || op == "or" || op == "condition" {
				return false
			}
			if !atomicShape(arg) {
				return false
			}
		}
	case []any:
		for _, arg := range v {
			if !atomicShape(arg) {
				return false
			}
		}
	}
	return true
}
