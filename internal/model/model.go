// Package model contains the serializable specification, independent of input formats and rendering.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type Project struct {
	SchemaVersion string             `json:"schemaVersion"`
	Name          string             `json:"name"`
	Repository    string             `json:"repository,omitempty"`
	Files         []string           `json:"files"`
	Output        string             `json:"output"`
	Limits        Limits             `json:"limits"`
	Coverage      map[string]float64 `json:"coverage"`
}
type Limits struct {
	Steps int `json:"steps"`
	Paths int `json:"paths"`
}

func (l *Limits) Defaults() {
	if l.Steps == 0 {
		l.Steps = 12
	}
	if l.Paths == 0 {
		l.Paths = 100000
	}
}

// Document is one source file. Its top level is the unnamed root state: start,
// events and states written here belong to the root state shared by all files.
type Document struct {
	SchemaVersion   string                    `json:"schemaVersion"`
	Start           *Target                   `json:"start,omitempty"`
	Actors          map[string]Actor          `json:"actors,omitempty"`
	Events          map[string]Event          `json:"events,omitempty"`
	States          map[string]State          `json:"states,omitempty"`
	UseCasePackages map[string]UseCasePackage `json:"usecasePackages,omitempty"`
	UseCases        map[string]UseCase        `json:"usecases,omitempty"`
	CommonRules     map[string]CommonRule     `json:"commonRules,omitempty"`
	Regressions     map[string]Scenario       `json:"regressions,omitempty"`
}
type Actor struct {
	Description string `json:"description,omitempty"`
}

// State nests its child states. A state with children must have a start.
type State struct {
	Screen      string           `json:"screen,omitempty"`
	LayoutLayer *int             `json:"layoutLayer,omitempty"`
	Start       *Target          `json:"start,omitempty"`
	Events      map[string]Event `json:"events,omitempty"`
	States      map[string]State `json:"states,omitempty"`
}

// Target is the common part of every transition: an action followed by a
// destination state OR a decision. Neither means staying in the current state
// (not allowed for a start).
type Target struct {
	Action   string    `json:"action,omitempty"`
	To       string    `json:"to,omitempty"`
	Decision *Decision `json:"decision,omitempty"`
}

// Event is a transition triggered by an actor in the state that declares it.
type Event struct {
	Actors         []string `json:"actors"`
	DefaultVisible bool     `json:"defaultVisible,omitempty"`
	Target
}

// Decision branches by name only; scenarios choose the branch to take.
type Decision struct {
	Label    string            `json:"label,omitempty"`
	Branches map[string]Target `json:"branches"`
}

// Transition kinds in the compiled index.
const (
	KindEvent  = "event"
	KindStart  = "start"
	KindBranch = "branch"
)

// StartName marks the start transition in transition IDs. Names cannot contain brackets.
const StartName = "[開始]"

// StateNode is a compiled state addressed by its path from the root ("" is the root).
type StateNode struct {
	Name        string            `json:"name"`
	Parent      string            `json:"parent"`
	Screen      string            `json:"screen,omitempty"`
	LayoutLayer *int              `json:"layoutLayer,omitempty"`
	Start       string            `json:"start,omitempty"`
	Children    []string          `json:"children,omitempty"`
	Events      map[string]string `json:"events,omitempty"`
}

// Transition is a compiled event, start or branch. From is a state path for
// events and starts and a decision ID for branches. To is a state path, a
// decision ID, or empty (stay in the state where the event was raised).
type Transition struct {
	Kind           string   `json:"kind"`
	From           string   `json:"from"`
	Name           string   `json:"name,omitempty"`
	To             string   `json:"to,omitempty"`
	Action         string   `json:"action,omitempty"`
	Actors         []string `json:"actors,omitempty"`
	DefaultVisible bool     `json:"defaultVisible"`
	// Root is the event or start transition that heads this decision tree.
	Root string `json:"root"`
}

// DecisionNode is an anonymous decision owned by the transition that enters it.
type DecisionNode struct {
	Label    string   `json:"label,omitempty"`
	Parent   string   `json:"parent"`
	Owner    string   `json:"owner"`
	Branches []string `json:"branches"`
}

type UseCasePackage struct {
	Name   string `json:"name"`
	Parent string `json:"parent,omitempty"`
}

// UseCase references states and transitions by reference strings. After
// compilation Starts hold state paths and Ends/Scope hold transition IDs.
type UseCase struct {
	Name            string                    `json:"name"`
	Goal            string                    `json:"goal"`
	Actor           string                    `json:"actor"`
	Package         string                    `json:"package,omitempty"`
	Starts          []string                  `json:"starts"`
	Ends            []string                  `json:"ends"`
	Scope           []string                  `json:"scope"`
	Scenarios       map[string]Scenario       `json:"scenarios"`
	CoverageReviews map[string]CoverageReview `json:"coverageReviews,omitempty"`
}

// Scenario steps name events and the branches taken after them. Regressions
// declare their own actor; use-case scenarios use the use-case actor.
type Scenario struct {
	Name            string `json:"name"`
	Goal            string `json:"goal"`
	ExpectedOutcome string `json:"expectedOutcome"`
	Actor           string `json:"actor,omitempty"`
	Start           string `json:"start"`
	Steps           []Step `json:"steps"`
	Description     string `json:"description,omitempty"`
}
type Step struct {
	Event    string   `json:"event"`
	Branches []string `json:"branches,omitempty"`
	Reject   bool     `json:"reject,omitempty"`
	Note     string   `json:"note,omitempty"`
}
type CoverageReview struct {
	Kind                     string   `json:"kind"`
	Requirements             []string `json:"requirements"`
	Status                   string   `json:"status"`
	Reason                   string   `json:"reason"`
	Scenario                 string   `json:"scenario,omitempty"`
	ArchitectureRequirements []string `json:"architectureRequirements,omitempty"`
	BasisHash                string   `json:"basisHash"`
}

// CommonRule describes product-wide behavior outside the state structure.
type CommonRule struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
type Source struct {
	File    string `json:"file"`
	Pointer string `json:"pointer,omitempty"`
	Line    int    `json:"line,omitempty"`
}

// Spec is the compiled specification shared by analysis and rendering.
type Spec struct {
	Project         Project                   `json:"project"`
	Actors          map[string]Actor          `json:"actors"`
	States          map[string]StateNode      `json:"states"`
	Transitions     map[string]Transition     `json:"transitions"`
	Decisions       map[string]DecisionNode   `json:"decisions"`
	UseCasePackages map[string]UseCasePackage `json:"usecasePackages"`
	UseCases        map[string]UseCase        `json:"usecases"`
	CommonRules     map[string]CommonRule     `json:"commonRules"`
	Regressions     map[string]Scenario       `json:"regressions"`
	Sources         map[string]Source         `json:"sources"`
	Hash            string                    `json:"hash"`
	// Problems found while compiling references; reported with validation.
	Problems []Diagnostic `json:"-"`
}

func NewSpec() *Spec {
	return &Spec{Actors: map[string]Actor{}, States: map[string]StateNode{"": {Events: map[string]string{}}}, Transitions: map[string]Transition{}, Decisions: map[string]DecisionNode{}, UseCasePackages: map[string]UseCasePackage{}, UseCases: map[string]UseCase{}, CommonRules: map[string]CommonRule{}, Regressions: map[string]Scenario{}, Sources: map[string]Source{}}
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

// ChildPath joins a state path and a child name.
func ChildPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "/" + name
}

// EventID, StartID, BranchID and DecisionID build stable transition identities.
func EventID(state, event string) string   { return state + ":" + event }
func StartID(state string) string          { return state + ":" + StartName }
func BranchID(owner, branch string) string { return owner + ">" + branch }
func DecisionID(owner string) string       { return owner + "?" }

func (s *Spec) IsDecision(id string) bool {
	_, ok := s.Decisions[id]
	return ok
}
func (s *Spec) IsLeaf(id string) bool {
	st, ok := s.States[id]
	return ok && id != "" && len(st.Children) == 0
}

// StatePath returns the state followed by its ancestors, ending at the root "".
func (s *Spec) StatePath(id string) []string {
	path := []string{}
	for {
		if _, ok := s.States[id]; !ok {
			return path
		}
		path = append(path, id)
		if id == "" {
			return path
		}
		id = s.States[id].Parent
	}
}

// InState reports whether the active leaf is the state or one of its descendants.
func (s *Spec) InState(active, id string) bool {
	return Contains(s.StatePath(active), id)
}

// Leaves gives the leaf states at or below a state.
func (s *Spec) Leaves(id string) []string {
	ids := []string{}
	for _, leaf := range Keys(s.States) {
		if s.IsLeaf(leaf) && s.InState(leaf, id) {
			ids = append(ids, leaf)
		}
	}
	return ids
}

// Enter follows start transitions into a state until a leaf or a start decision.
func (s *Spec) Enter(id string) (string, error) {
	seen := map[string]bool{}
	for {
		if s.IsDecision(id) {
			return id, nil
		}
		st, ok := s.States[id]
		if !ok {
			return "", fmt.Errorf("unknown state %q", id)
		}
		if id != "" && len(st.Children) == 0 {
			return id, nil
		}
		if seen[id] {
			return "", fmt.Errorf("start of %q enters itself", id)
		}
		seen[id] = true
		t, ok := s.Transitions[st.Start]
		if !ok || t.To == "" {
			return "", fmt.Errorf("state %q has no start", Display(id))
		}
		id = t.To
	}
}

// Display gives a readable state path; the root is shown explicitly.
func Display(path string) string {
	if path == "" {
		return "(ルート)"
	}
	return path
}

// Label is the last name of a state path, or the decision label.
func (s *Spec) Label(id string) string {
	if d, ok := s.Decisions[id]; ok {
		if d.Label != "" {
			return d.Label
		}
		return "ディシジョン"
	}
	if st, ok := s.States[id]; ok {
		if id == "" {
			return Display(id)
		}
		return st.Name
	}
	return id
}

// Snapshot is a control configuration: an active leaf, or a decision being
// resolved together with the leaf that raised the event (for staying branches).
type Snapshot struct {
	State  string `json:"state"`
	Origin string `json:"origin,omitempty"`
}

// Witness is a start leaf and the transition IDs taken from it.
type Witness struct {
	Start       string   `json:"start"`
	Transitions []string `json:"transitions"`
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
	State   string   `json:"state,omitempty"`
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
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Goal            string     `json:"goal"`
	ExpectedOutcome string     `json:"expectedOutcome"`
	UseCase         string     `json:"usecase,omitempty"`
	Actor           string     `json:"actor"`
	Steps           []Step     `json:"steps"`
	Valid           bool       `json:"valid"`
	Error           string     `json:"error,omitempty"`
	Trace           []Snapshot `json:"trace"`
	Witness         Witness    `json:"witness"`
}
type UseCaseResult struct {
	ID              string                 `json:"id"`
	Coverage        []Coverage             `json:"coverage"`
	ReachableStates int                    `json:"reachableStates"`
	Complete        bool                   `json:"complete"`
	ReviewBasisHash string                 `json:"reviewBasisHash"`
	CoverageReviews []CoverageReviewResult `json:"coverageReviews"`
}
type CoverageReviewResult struct {
	ID     string         `json:"id"`
	Review CoverageReview `json:"review"`
	Status string         `json:"status"`
	Error  string         `json:"error,omitempty"`
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

var CoverageKinds = []string{"states", "compositeStates", "transitions", "pairs", "primePaths", "boundedPaths"}

// ForbiddenNameRunes cannot appear in actor, state, event or branch names
// because they separate parts of references and transition IDs.
const ForbiddenNameRunes = "/:>?[]"

func ValidName(name string) bool {
	return name != "" && strings.TrimSpace(name) == name && !strings.ContainsAny(name, ForbiddenNameRunes)
}
