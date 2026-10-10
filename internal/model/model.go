// Package model contains the serializable specification, independent of input formats and rendering.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

type Expr = any

type Project struct {
	SchemaVersion string             `json:"schemaVersion"`
	Name          string             `json:"name"`
	Repository    string             `json:"repository,omitempty"`
	Files         []string           `json:"files"`
	Output        string             `json:"output"`
	Initial       []Snapshot         `json:"initial"`
	Limits        Limits             `json:"limits"`
	Coverage      map[string]float64 `json:"coverage"`
}
type Limits struct {
	States       int `json:"states"`
	Steps        int `json:"steps"`
	Paths        int `json:"paths"`
	Combinations int `json:"combinations"`
}

func (l *Limits) Defaults() {
	if l.States == 0 {
		l.States = 10000
	}
	if l.Steps == 0 {
		l.Steps = 12
	}
	if l.Paths == 0 {
		l.Paths = 100000
	}
	if l.Combinations == 0 {
		l.Combinations = 4096
	}
}

type Document struct {
	SchemaVersion string                   `json:"schemaVersion"`
	Variables     map[string]Variable      `json:"variables,omitempty"`
	Screens       map[string]Screen        `json:"screens,omitempty"`
	States        map[string]State         `json:"states,omitempty"`
	Events        map[string]Event         `json:"events,omitempty"`
	Conditions    map[string]Expr          `json:"conditions,omitempty"`
	Transitions   map[string]Transition    `json:"transitions,omitempty"`
	UseCases      map[string]UseCase       `json:"usecases,omitempty"`
	Properties    map[string]Property      `json:"properties,omitempty"`
	Decisions     map[string]DecisionTable `json:"decisions,omitempty"`
	Regressions   map[string]Scenario      `json:"regressions,omitempty"`
}
type Variable struct {
	Domain      []any  `json:"domain"`
	Initial     any    `json:"initial"`
	Description string `json:"description,omitempty"`
}
type Screen struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}
type State struct {
	Label    string   `json:"label"`
	Screen   string   `json:"screen"`
	Events   []string `json:"events"`
	Terminal bool     `json:"terminal,omitempty"`
}
type Event struct {
	Label      string           `json:"label"`
	Source     string           `json:"source"`
	Parameters map[string][]any `json:"parameters,omitempty"`
	When       Expr             `json:"when,omitempty"`
}
type Transition struct {
	From          string          `json:"from"`
	To            string          `json:"to"`
	Event         string          `json:"event"`
	Guard         Expr            `json:"guard,omitempty"`
	Action        string          `json:"action"`
	Set           map[string]Expr `json:"set,omitempty"`
	UnownedReason string          `json:"unownedReason,omitempty"`
}
type Snapshot struct {
	State  string         `json:"state"`
	Values map[string]any `json:"values,omitempty"`
}
type Step struct {
	Transition string         `json:"transition"`
	Input      map[string]any `json:"input,omitempty"`
	Reject     bool           `json:"reject,omitempty"`
	Expect     Expr           `json:"expect,omitempty"`
	Note       string         `json:"note,omitempty"`
}
type Scenario struct {
	Name        string   `json:"name"`
	Start       Snapshot `json:"start"`
	Steps       []Step   `json:"steps"`
	Expect      Expr     `json:"expect"`
	Description string   `json:"description,omitempty"`
}
type UseCase struct {
	Name      string              `json:"name"`
	Goal      string              `json:"goal"`
	Actor     string              `json:"actor"`
	Starts    []Snapshot          `json:"starts"`
	End       Expr                `json:"end"`
	Scope     []string            `json:"scope"`
	Scenarios map[string]Scenario `json:"scenarios"`
}
type Property struct {
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Assert      Expr   `json:"assert"`
}
type DecisionTable struct {
	State string         `json:"state"`
	Event string         `json:"event"`
	Cases []DecisionCase `json:"cases"`
}
type DecisionCase struct {
	Values  map[string]any `json:"values,omitempty"`
	Input   map[string]any `json:"input,omitempty"`
	Enabled []string       `json:"enabled"`
}
type Source struct {
	File    string `json:"file"`
	Pointer string `json:"pointer,omitempty"`
	Line    int    `json:"line,omitempty"`
}
type Spec struct {
	Project Project `json:"project"`
	Document
	Sources map[string]Source `json:"sources"`
	Hash    string            `json:"hash"`
}

func EmptyDocument() Document {
	return Document{SchemaVersion: "1", Variables: map[string]Variable{}, Screens: map[string]Screen{}, States: map[string]State{}, Events: map[string]Event{}, Conditions: map[string]Expr{}, Transitions: map[string]Transition{}, UseCases: map[string]UseCase{}, Properties: map[string]Property{}, Decisions: map[string]DecisionTable{}, Regressions: map[string]Scenario{}}
}
func Keys[V any](m map[string]V) []string {
	k := make([]string, 0, len(m))
	for s := range m {
		k = append(k, s)
	}
	sort.Strings(k)
	return k
}
func JSON(v any) string {
	b, e := json.Marshal(v)
	if e != nil {
		panic(e)
	}
	return string(b)
}
func Hash(v any) string   { s := sha256.Sum256([]byte(JSON(v))); return hex.EncodeToString(s[:]) }
func Equal(a, b any) bool { return JSON(a) == JSON(b) }
func Contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func CopyValues(a map[string]any) map[string]any {
	b := map[string]any{}
	for k, v := range a {
		b[k] = v
	}
	return b
}
func CloneSnapshot(s Snapshot) Snapshot {
	return Snapshot{State: s.State, Values: CopyValues(s.Values)}
}
func (s *Spec) Normalize(v Snapshot) (Snapshot, error) {
	if _, ok := s.States[v.State]; !ok {
		return v, fmt.Errorf("unknown state %q", v.State)
	}
	r := Snapshot{State: v.State, Values: map[string]any{}}
	for k, d := range s.Variables {
		r.Values[k] = d.Initial
	}
	for _, k := range Keys(v.Values) {
		x := v.Values[k]
		d, ok := s.Variables[k]
		if !ok {
			return v, fmt.Errorf("unknown variable %q", k)
		}
		if !InDomain(d.Domain, x) {
			return v, fmt.Errorf("%s=%v outside domain", k, x)
		}
		r.Values[k] = x
	}
	return r, nil
}
func InDomain(domain []any, value any) bool {
	for _, x := range domain {
		if Equal(x, value) {
			return true
		}
	}
	return false
}

// Diagnostic paths include concrete event parameters and their initial data.
type Witness struct {
	Start Snapshot `json:"start"`
	Steps []Step   `json:"steps"`
}
type Diagnostic struct {
	Code     string   `json:"code"`
	Severity string   `json:"severity"`
	Subject  string   `json:"subject,omitempty"`
	Message  string   `json:"message"`
	Source   Source   `json:"source,omitempty"`
	Witness  *Witness `json:"witness,omitempty"`
}
type Check struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Status  string   `json:"status"`
	Message string   `json:"message"`
	Witness *Witness `json:"witness,omitempty"`
}
type Requirement struct {
	ID      string   `json:"id"`
	Path    []string `json:"path,omitempty"`
	Witness *Witness `json:"witness,omitempty"`
}
type Coverage struct {
	Kind       string        `json:"kind"`
	Covered    int           `json:"covered"`
	Total      int           `json:"total"`
	Percent    *float64      `json:"percent"`
	Complete   bool          `json:"complete"`
	Missing    []Requirement `json:"missing"`
	Infeasible []Requirement `json:"infeasible,omitempty"`
	Note       string        `json:"note"`
}
type ScenarioResult struct {
	Expect            Expr          `json:"expect"`
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	UseCase           string        `json:"usecase,omitempty"`
	Valid             bool          `json:"valid"`
	Error             string        `json:"error,omitempty"`
	Trace             []Snapshot    `json:"trace"`
	Witness           Witness       `json:"witness"`
	GuardObservations []Observation `json:"guardObservations,omitempty"`
}
type Observation struct {
	Transition string          `json:"transition"`
	Atoms      map[string]bool `json:"atoms"`
	Guard      bool            `json:"guard"`
}
type UseCaseResult struct {
	ID              string     `json:"id"`
	Coverage        []Coverage `json:"coverage"`
	ReachableStates int        `json:"reachableStates"`
	Complete        bool       `json:"complete"`
}
type Impact struct {
	Changed              []string           `json:"changed"`
	AffectedUseCases     []string           `json:"affectedUseCases"`
	InvalidatedScenarios []string           `json:"invalidatedScenarios"`
	CoverageChanges      map[string]float64 `json:"coverageChanges"`
}
type Report struct {
	FormatVersion   string              `json:"formatVersion"`
	ToolVersion     string              `json:"toolVersion"`
	SchemaVersion   string              `json:"schemaVersion"`
	Name            string              `json:"name"`
	InputHash       string              `json:"inputHash"`
	Limits          Limits              `json:"limits"`
	Status          string              `json:"status"`
	Complete        bool                `json:"complete"`
	ReachableStates int                 `json:"reachableStates"`
	Diagnostics     []Diagnostic        `json:"diagnostics"`
	Checks          []Check             `json:"checks"`
	Scenarios       []ScenarioResult    `json:"scenarios"`
	UseCases        []UseCaseResult     `json:"usecases"`
	Definitions     map[string]string   `json:"definitions"`
	Dependencies    map[string][]string `json:"dependencies"`
	Impact          *Impact             `json:"impact,omitempty"`
}

var CoverageKinds = []string{"transitions", "pairs", "primePaths", "boundedPaths", "conditions", "multipleConditions"}
