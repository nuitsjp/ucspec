// Package compiler resolves source files into a single, deterministically ordered model.
package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/nuitsjp/ucspec/internal/model"
)

type Codec struct {
	Decode   func([]byte, string) ([]byte, error)
	Validate func(string, any) error
	// Normalize folds names that look the same (for example NFKC). Names whose
	// normalized forms collide are rejected. Nil compares names as written.
	Normalize func(string) string
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

// source remembers where a root-level definition came from.
type source struct {
	file    string
	pointer string
}

// merged is the union of all source files before compilation.
type merged struct {
	root     model.State
	sources  map[string]source
	actors   map[string]model.Actor
	packages map[string]model.UseCasePackage
	usecases map[string]model.UseCase
	rules    map[string]model.CommonRule
	regs     map[string]model.Scenario
}

// Load reads the project and every source file, then compiles one model.
// Structural problems found while compiling are reported by Validate.
func Load(path string, c Codec) (*model.Spec, error) {
	p, err := ReadProject(path, c)
	if err != nil {
		return nil, err
	}
	s := model.NewSpec()
	s.Project = p
	root, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return s, err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return s, err
	}
	all := map[string]bool{}
	for _, pattern := range p.Files {
		if filepath.IsAbs(pattern) || strings.Contains(filepath.ToSlash(pattern), "../") || pattern == ".." {
			return s, fmt.Errorf("source pattern must stay in spec directory: %s", pattern)
		}
		matches, er := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		if er != nil {
			return s, er
		}
		if len(matches) == 0 {
			return s, fmt.Errorf("source pattern matched no files: %s", pattern)
		}
		for _, m := range matches {
			if all[m] {
				return s, fmt.Errorf("source included more than once: %s", m)
			}
			all[m] = true
		}
	}
	docs := map[string]model.Document{}
	for _, name := range model.Keys(all) {
		real, er := filepath.EvalSymlinks(name)
		if er != nil {
			return s, er
		}
		rel, er := filepath.Rel(realRoot, real)
		if er != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return s, fmt.Errorf("source escapes spec directory: %s", name)
		}
		b, er := os.ReadFile(name)
		if er != nil {
			return s, er
		}
		var doc model.Document
		if er = decode(c, b, name, "document", &doc); er != nil {
			return s, fmt.Errorf("%s: %w", name, er)
		}
		if doc.SchemaVersion != "1" {
			return s, fmt.Errorf("%s: unsupported schemaVersion %q", name, doc.SchemaVersion)
		}
		display, _ := filepath.Rel(root, name)
		docs[filepath.ToSlash(display)] = doc
	}
	return FromDocuments(p, docs, c.Normalize)
}

// FromDocuments compiles documents keyed by file name, in name order.
func FromDocuments(p model.Project, docs map[string]model.Document, normalize func(string) string) (*model.Spec, error) {
	s := model.NewSpec()
	s.Project = p
	m := &merged{root: model.State{Events: map[string]model.Event{}, States: map[string]model.State{}}, sources: map[string]source{}, actors: map[string]model.Actor{}, packages: map[string]model.UseCasePackage{}, usecases: map[string]model.UseCase{}, rules: map[string]model.CommonRule{}, regs: map[string]model.Scenario{}}
	for _, name := range model.Keys(docs) {
		if err := m.add(docs[name], name); err != nil {
			return s, err
		}
	}
	s.Hash = model.Hash(map[string]any{"project": p, "root": m.root, "actors": m.actors, "usecasePackages": m.packages, "usecases": m.usecases, "commonRules": m.rules, "regressions": m.regs})
	compile(s, m, normalize)
	return s, nil
}

func (m *merged) add(doc model.Document, file string) error {
	put := func(section, key string) error {
		id := section + "/" + key
		if previous, ok := m.sources[id]; ok {
			return fmt.Errorf("duplicate %s in %s (first: %s)", id, file, previous.file)
		}
		m.sources[id] = source{file, "/" + section + "/" + escape(key)}
		return nil
	}
	if doc.Start != nil {
		if previous, ok := m.sources["start"]; ok {
			return fmt.Errorf("duplicate root start in %s (first: %s)", file, previous.file)
		}
		m.sources["start"] = source{file, "/start"}
		m.root.Start = doc.Start
	}
	for _, k := range model.Keys(doc.Actors) {
		if err := put("actors", k); err != nil {
			return err
		}
		m.actors[k] = doc.Actors[k]
	}
	for _, k := range model.Keys(doc.Events) {
		if err := put("events", k); err != nil {
			return err
		}
		m.root.Events[k] = doc.Events[k]
	}
	for _, k := range model.Keys(doc.States) {
		if err := put("states", k); err != nil {
			return err
		}
		m.root.States[k] = doc.States[k]
	}
	for _, k := range model.Keys(doc.UseCasePackages) {
		if err := put("usecasePackages", k); err != nil {
			return err
		}
		m.packages[k] = doc.UseCasePackages[k]
	}
	for _, k := range model.Keys(doc.UseCases) {
		if err := put("usecases", k); err != nil {
			return err
		}
		m.usecases[k] = doc.UseCases[k]
	}
	for _, k := range model.Keys(doc.CommonRules) {
		if err := put("commonRules", k); err != nil {
			return err
		}
		m.rules[k] = doc.CommonRules[k]
	}
	for _, k := range model.Keys(doc.Regressions) {
		if err := put("regressions", k); err != nil {
			return err
		}
		m.regs[k] = doc.Regressions[k]
	}
	return nil
}
func escape(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }

// compiler state while flattening the nested model.
type compilation struct {
	s         *model.Spec
	normalize func(string) string
	problems  []model.Diagnostic
}

func (c *compilation) problem(subject, message string) {
	c.problems = append(c.problems, model.Diagnostic{Code: "structure", Severity: "error", Subject: subject, Message: message, Source: c.s.Sources[subject]})
}

func compile(s *model.Spec, m *merged, normalize func(string) string) {
	c := &compilation{s: s, normalize: normalize}
	for _, id := range model.Keys(m.sources) {
		src := m.sources[id]
		s.Sources[id] = model.Source{File: src.file, Pointer: src.pointer}
	}
	s.Actors = m.actors
	s.UseCasePackages = m.packages
	s.CommonRules = m.rules
	c.names("actors", "", model.Keys(m.actors))
	// Pass 1: state paths, so that destinations can refer to any state.
	rootFiles := map[string]source{}
	for _, name := range model.Keys(m.root.States) {
		rootFiles[name] = m.sources["states/"+name]
	}
	c.collect("", m.root, "", rootFiles)
	// Pass 2: transitions and decisions.
	c.transitions("", m.root, m.sources["start"], m.sources)
	usecases := map[string]model.UseCase{}
	for _, id := range model.Keys(m.usecases) {
		usecases[id] = c.usecase(id, m.usecases[id])
	}
	s.UseCases = usecases
	regs := map[string]model.Scenario{}
	for _, id := range model.Keys(m.regs) {
		sc := m.regs[id]
		sc.Start = c.stateRef("regressions/"+id, sc.Start)
		regs[id] = sc
	}
	s.Regressions = regs
	s.Problems = c.problems
}

// collect registers the state at path and its descendants.
func (c *compilation) collect(path string, st model.State, file string, rootFiles map[string]source) {
	node := model.StateNode{Name: "", Screen: st.Screen, LayoutLayer: st.LayoutLayer, Events: map[string]string{}}
	if path != "" {
		parts := strings.Split(path, "/")
		node.Name = parts[len(parts)-1]
		node.Parent = strings.Join(parts[:len(parts)-1], "/")
	}
	names := model.Keys(st.States)
	c.names("states", path, names)
	for _, name := range names {
		child := model.ChildPath(path, name)
		node.Children = append(node.Children, child)
		f := file
		pointer := ""
		if path == "" {
			f = rootFiles[name].file
			pointer = rootFiles[name].pointer
		} else {
			pointer = c.s.Sources["states/"+path].Pointer + "/states/" + escape(name)
		}
		c.s.Sources["states/"+child] = model.Source{File: f, Pointer: pointer}
		c.collect(child, st.States[name], f, rootFiles)
	}
	c.s.States[path] = node
}

func (c *compilation) transitions(path string, st model.State, start source, rootSources map[string]source) {
	node := c.s.States[path]
	subject := "states/" + path
	pointer := c.s.Sources[subject].Pointer
	file := c.s.Sources[subject].File
	if path == "" {
		subject = "start"
	}
	if st.Start != nil {
		id := model.StartID(path)
		src := model.Source{File: file, Pointer: pointer + "/start"}
		if path == "" {
			src = model.Source{File: start.file, Pointer: start.pointer}
		}
		c.s.Sources["transitions/"+id] = src
		node.Start = id
		t := model.Transition{Kind: model.KindStart, From: path, Root: id, DefaultVisible: true}
		c.target(&t, id, *st.Start, path, path, true, src)
		if len(node.Children) == 0 {
			c.problem("transitions/"+id, "a leaf state must not have a start")
		}
	} else if len(node.Children) > 0 {
		c.problem(subject, "a state with child states requires a start")
	}
	events := model.Keys(st.Events)
	c.names("events", path, events)
	for _, name := range events {
		ev := st.Events[name]
		id := model.EventID(path, name)
		src := model.Source{File: file, Pointer: pointer + "/events/" + escape(name)}
		if path == "" {
			r := rootSources["events/"+name]
			src = model.Source{File: r.file, Pointer: r.pointer}
		}
		c.s.Sources["transitions/"+id] = src
		node.Events[name] = id
		t := model.Transition{Kind: model.KindEvent, From: path, Name: name, Actors: ev.Actors, DefaultVisible: ev.DefaultVisible, Root: id}
		if len(ev.Actors) == 0 {
			c.problem("transitions/"+id, "event requires at least one actor")
		}
		for _, actor := range ev.Actors {
			if _, ok := c.s.Actors[actor]; !ok {
				c.problem("transitions/"+id, "unknown actor "+actor)
			}
		}
		parent := node.Parent
		if path == "" {
			parent = ""
		}
		c.target(&t, id, ev.Target, parent, "", ev.DefaultVisible, src)
	}
	c.s.States[path] = node
	for _, child := range node.Children {
		name := c.s.States[child].Name
		c.transitions(child, st.States[name], start, rootSources)
	}
}

// target fills the destination of t and compiles its decision tree. For a
// start tree, owner is the composite whose start it is: every destination must
// lie inside it and no branch may stay. owner is "" for event trees.
func (c *compilation) target(t *model.Transition, id string, tg model.Target, decisionParent, owner string, visible bool, src model.Source) {
	subject := "transitions/" + id
	t.Action = tg.Action
	// Event names cannot contain brackets, so only start trees have this root.
	isStart := strings.HasSuffix(t.Root, ":"+model.StartName)
	switch {
	case tg.To != "" && tg.Decision != nil:
		c.problem(subject, "to and decision are exclusive")
	case tg.To != "":
		t.To = c.stateRef(subject, tg.To)
		if isStart && t.To != "" {
			if owner != "" && (t.To == owner || !c.s.InState(t.To, owner)) {
				c.problem(subject, "a start must enter a state inside "+owner)
			}
		}
	case tg.Decision != nil:
		did := model.DecisionID(id)
		d := model.DecisionNode{Label: tg.Decision.Label, Parent: decisionParent, Owner: id}
		names := model.Keys(tg.Decision.Branches)
		c.names("branches", id, names)
		if len(names) < 2 {
			c.problem(subject, "a decision requires at least two branches")
		}
		for _, name := range names {
			d.Branches = append(d.Branches, model.BranchID(id, name))
		}
		c.s.Decisions[did] = d
		t.To = did
		for _, name := range names {
			bid := model.BranchID(id, name)
			bsrc := model.Source{File: src.File, Pointer: src.Pointer + "/decision/branches/" + escape(name)}
			c.s.Sources["transitions/"+bid] = bsrc
			b := model.Transition{Kind: model.KindBranch, From: did, Name: name, Root: t.Root, DefaultVisible: visible}
			c.target(&b, bid, tg.Decision.Branches[name], decisionParent, owner, visible, bsrc)
		}
	default:
		if isStart {
			c.problem(subject, "a start must have a destination or a decision")
		}
	}
	c.s.Transitions[id] = *t
}

// names checks the names of one kind of sibling definitions.
func (c *compilation) names(kind, owner string, names []string) {
	seen := map[string]string{}
	for _, name := range names {
		subject := kind + "/" + model.ChildPath(owner, name)
		if !model.ValidName(name) {
			c.problem(subject, "name must be non-empty, without surrounding spaces and without "+model.ForbiddenNameRunes)
		}
		key := name
		if c.normalize != nil {
			key = c.normalize(name)
		}
		if previous, ok := seen[key]; ok {
			c.problem(subject, fmt.Sprintf("name looks the same as %q", previous))
		}
		seen[key] = name
	}
}

// stateRef resolves a state reference: a full path or a unique path suffix.
func (c *compilation) stateRef(subject, ref string) string {
	path, err := ResolveState(c.s, ref)
	if err != nil {
		c.problem(subject, err.Error())
		return ""
	}
	return path
}

// ResolveState accepts a full path or a path suffix that names exactly one state.
func ResolveState(s *model.Spec, ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("state reference is empty")
	}
	matches := []string{}
	for _, path := range model.Keys(s.States) {
		if path != "" && (path == ref || strings.HasSuffix(path, "/"+ref)) {
			matches = append(matches, path)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("unknown state %q", ref)
	default:
		return "", fmt.Errorf("ambiguous state %q matches %s; write more of the path", ref, strings.Join(matches, ", "))
	}
}

// ResolveTransition accepts "state:event", "state:event>branch>…" and
// "state:[開始]>branch…". An empty state part means the root state.
func ResolveTransition(s *model.Spec, ref string) (string, error) {
	statePart, rest, ok := strings.Cut(ref, ":")
	if !ok || rest == "" {
		return "", fmt.Errorf("transition reference %q must be state:event", ref)
	}
	state := ""
	if statePart != "" {
		var err error
		if state, err = ResolveState(s, statePart); err != nil {
			return "", err
		}
	}
	parts := strings.Split(rest, ">")
	var id string
	if parts[0] == model.StartName {
		id = s.States[state].Start
		if id == "" {
			return "", fmt.Errorf("state %q has no start", model.Display(state))
		}
	} else {
		id = model.EventID(state, parts[0])
		if t, ok := s.Transitions[id]; !ok || t.Kind != model.KindEvent {
			return "", fmt.Errorf("state %q declares no event %q", model.Display(state), parts[0])
		}
	}
	for _, branch := range parts[1:] {
		if !s.IsDecision(model.DecisionID(id)) {
			return "", fmt.Errorf("%q has no decision for branch %q", id, branch)
		}
		id = model.BranchID(id, branch)
		if _, ok := s.Transitions[id]; !ok {
			return "", fmt.Errorf("unknown branch %q", branch)
		}
	}
	return id, nil
}

func (c *compilation) usecase(id string, u model.UseCase) model.UseCase {
	subject := "usecases/" + id
	starts := []string{}
	for _, ref := range u.Starts {
		starts = append(starts, c.stateRef(subject, ref))
	}
	u.Starts = starts
	ends := []string{}
	for _, ref := range u.Ends {
		t, err := ResolveTransition(c.s, ref)
		if err != nil {
			c.problem(subject, "ends: "+err.Error())
		}
		ends = append(ends, t)
	}
	u.Ends = ends
	scope := []string{}
	for _, ref := range u.Scope {
		t, err := ResolveTransition(c.s, ref)
		if err == nil && c.s.Transitions[t].Kind != model.KindEvent {
			err = fmt.Errorf("scope lists events; %q is not an event", ref)
		}
		if err != nil {
			c.problem(subject, "scope: "+err.Error())
		}
		scope = append(scope, t)
	}
	u.Scope = scope
	scenarios := map[string]model.Scenario{}
	for _, sid := range model.Keys(u.Scenarios) {
		sc := u.Scenarios[sid]
		sc.Start = c.stateRef(subject, sc.Start)
		scenarios[sid] = sc
	}
	u.Scenarios = scenarios
	return u
}

var idRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)

func Validate(s *model.Spec) []model.Diagnostic {
	d := append([]model.Diagnostic{}, s.Problems...)
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
	if p.Output == "" {
		add("project", "output is required")
	}
	l := p.Limits
	if l.Steps < 1 || l.Paths < 1 || l.Steps > 256 || l.Paths > 2000000 {
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
	if len(s.States) <= 1 || len(s.UseCases) == 0 {
		add("project", "at least one state and one use case are required")
	}
	if s.States[""].Start == "" {
		add("start", "the root requires a start (write start at the top level of one file)")
	}
	for _, id := range model.Keys(s.States) {
		if st := s.States[id]; st.LayoutLayer != nil && *st.LayoutLayer < 0 {
			add("states/"+id, "layoutLayer must be non-negative")
		}
	}
	validateUseCasePackages(s, add)
	for _, id := range model.Keys(s.CommonRules) {
		rule := s.CommonRules[id]
		if strings.TrimSpace(rule.Name) == "" || strings.TrimSpace(rule.Description) == "" {
			add("commonRules/"+id, "common rule name and description are required")
		}
		if !idRE.MatchString(id) {
			add("commonRules/"+id, "ID must match "+idRE.String())
		}
	}
	checkScenario := func(sub string, sc model.Scenario) {
		if sc.Name == "" {
			add(sub, "scenario name required")
		}
		if strings.TrimSpace(sc.Goal) == "" {
			add(sub, "scenario goal required")
		}
		if strings.TrimSpace(sc.ExpectedOutcome) == "" {
			add(sub, "scenario expectedOutcome required")
		}
		if len(sc.Steps) == 0 {
			add(sub, "scenario steps required")
		}
		for _, step := range sc.Steps {
			if step.Reject && len(step.Branches) > 0 {
				add(sub, "a rejected step must not choose branches")
			}
		}
	}
	for _, id := range model.Keys(s.UseCases) {
		u := s.UseCases[id]
		sub := "usecases/" + id
		if !idRE.MatchString(id) {
			add(sub, "ID must match "+idRE.String())
		}
		if u.Name == "" || u.Goal == "" || u.Actor == "" {
			add(sub, "name, goal and actor are required")
		}
		if _, ok := s.Actors[u.Actor]; u.Actor != "" && !ok {
			add(sub, "unknown actor "+u.Actor)
		}
		if len(u.Starts) == 0 || len(u.Ends) == 0 || len(u.Scope) == 0 {
			add(sub, "starts, ends and scope are required")
		}
		seen := map[string]bool{}
		for _, tr := range u.Scope {
			if tr == "" {
				continue
			}
			if seen[tr] {
				add(sub, "duplicate scope event "+tr)
			}
			seen[tr] = true
			if t := s.Transitions[tr]; !model.Contains(t.Actors, u.Actor) {
				add(sub, "scope event "+tr+" cannot be raised by "+u.Actor)
			}
		}
		for _, end := range u.Ends {
			if t, ok := s.Transitions[end]; ok && t.Kind != model.KindStart && s.Transitions[t.Root].Kind == model.KindEvent && !seen[t.Root] {
				add(sub, "end "+end+" belongs to an event outside the scope")
			}
		}
		for _, sid := range model.Keys(u.Scenarios) {
			sc := u.Scenarios[sid]
			if !idRE.MatchString(sid) {
				add(sub, "invalid scenario ID "+sid)
			}
			if sc.Actor != "" {
				add(sub, sid+": use-case scenarios use the use-case actor; remove actor")
			}
			checkScenario(sub, sc)
		}
		for _, message := range validateCoverageReviews(u) {
			add(sub, message)
		}
	}
	for _, id := range model.Keys(s.Regressions) {
		sub := "regressions/" + id
		sc := s.Regressions[id]
		if !idRE.MatchString(id) {
			add(sub, "ID must match "+idRE.String())
		}
		if _, ok := s.Actors[sc.Actor]; !ok {
			add(sub, "regressions require a known actor")
		}
		checkScenario(sub, sc)
	}
	sort.SliceStable(d, func(i, j int) bool { return d[i].Subject+d[i].Message < d[j].Subject+d[j].Message })
	return d
}

func validateUseCasePackages(s *model.Spec, add func(string, string)) {
	for _, id := range model.Keys(s.UseCasePackages) {
		pkg := s.UseCasePackages[id]
		sub := "usecasePackages/" + id
		if !idRE.MatchString(id) {
			add(sub, "ID must match "+idRE.String())
		}
		if pkg.Name == "" {
			add(sub, "name required")
		}
		if pkg.Parent != "" {
			if _, ok := s.UseCasePackages[pkg.Parent]; !ok {
				add(sub, "unknown parent package "+pkg.Parent)
			} else if pkg.Parent == id {
				add(sub, "package cannot be its own parent")
			}
		}
		seen := map[string]bool{}
		for current := id; current != ""; {
			if seen[current] {
				add(sub, "package hierarchy contains a cycle")
				break
			}
			seen[current] = true
			ancestor, ok := s.UseCasePackages[current]
			if !ok {
				break
			}
			current = ancestor.Parent
		}
	}
	for _, id := range model.Keys(s.UseCases) {
		pkg := s.UseCases[id].Package
		if pkg != "" {
			if _, ok := s.UseCasePackages[pkg]; !ok {
				add("usecases/"+id, "unknown use-case package "+pkg)
			}
		}
	}
}
