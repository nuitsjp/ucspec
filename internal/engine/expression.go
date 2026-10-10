package engine

import (
	"encoding/json"
	"fmt"
	"github.com/nuitsjp/usecase-spec/internal/model"
)

// Expressions are data, never executable user code. Boolean operands are evaluated
// eagerly to record logical condition truth, NOT language-level short-circuit coverage.
type Evaluation struct {
	Spec   *model.Spec
	State  model.Snapshot
	Input  map[string]any
	Atoms  map[string]bool
	active map[string]bool
}

func (e *Evaluation) Eval(x any) (any, error) {
	if x == nil {
		return nil, fmt.Errorf("null expression")
	}
	m, isMap := x.(map[string]any)
	if !isMap {
		switch x.(type) {
		case string, bool, float64, int, json.Number:
			return x, nil
		default:
			return nil, fmt.Errorf("invalid literal %T", x)
		}
	}
	if len(m) != 1 {
		return nil, fmt.Errorf("expression must have exactly one operator")
	}
	for op, arg := range m {
		switch op {
		case "var":
			k, ok := arg.(string)
			if !ok {
				return nil, fmt.Errorf("var name must be string")
			}
			if k == "$state" {
				return e.State.State, nil
			}
			v, ok := e.State.Values[k]
			if !ok {
				return nil, fmt.Errorf("unknown variable %q", k)
			}
			return v, nil
		case "param":
			k, ok := arg.(string)
			if !ok {
				return nil, fmt.Errorf("param name must be string")
			}
			v, ok := e.Input[k]
			if !ok {
				return nil, fmt.Errorf("missing event parameter %q", k)
			}
			return v, nil
		case "condition":
			k, ok := arg.(string)
			if !ok {
				return nil, fmt.Errorf("condition name must be string")
			}
			c, ok := e.Spec.Conditions[k]
			if !ok {
				return nil, fmt.Errorf("unknown condition %q", k)
			}
			if e.active == nil {
				e.active = map[string]bool{}
			}
			if e.active[k] {
				return nil, fmt.Errorf("condition reference cycle: %s", k)
			}
			e.active[k] = true
			v, err := e.Bool(c)
			delete(e.active, k)
			if err == nil && e.Atoms != nil {
				e.Atoms[k] = v
			}
			return v, err
		case "not":
			v, err := e.Bool(arg)
			return !v, err
		case "and", "or":
			args, ok := arg.([]any)
			if !ok || len(args) == 0 {
				return nil, fmt.Errorf("%s needs a nonempty list", op)
			}
			r := op == "and"
			for _, a := range args {
				v, err := e.Bool(a)
				if err != nil {
					return nil, err
				}
				if op == "and" {
					r = r && v
				} else {
					r = r || v
				}
			}
			return r, nil
		case "eq", "ne", "lt", "le", "gt", "ge":
			a, ok := arg.([]any)
			if !ok || len(a) != 2 {
				return nil, fmt.Errorf("%s needs two operands", op)
			}
			v, err := e.Eval(a[0])
			if err != nil {
				return nil, err
			}
			w, err := e.Eval(a[1])
			if err != nil {
				return nil, err
			}
			if op == "eq" {
				return model.Equal(v, w), nil
			}
			if op == "ne" {
				return !model.Equal(v, w), nil
			}
			n, ok := number(v)
			q, ok2 := number(w)
			if !ok || !ok2 {
				return nil, fmt.Errorf("%s needs numbers", op)
			}
			switch op {
			case "lt":
				return n < q, nil
			case "le":
				return n <= q, nil
			case "gt":
				return n > q, nil
			default:
				return n >= q, nil
			}
		default:
			return nil, fmt.Errorf("unknown expression operator %q", op)
		}
	}
	panic("unreachable")
}
func (e *Evaluation) Bool(x any) (bool, error) {
	v, err := e.Eval(x)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("boolean expression returned %T", v)
	}
	return b, nil
}
func number(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case json.Number:
		n, e := x.Float64()
		return n, e == nil
	}
	return 0, false
}
func Predicate(s *model.Spec, st model.Snapshot, x any) (bool, error) {
	return (&Evaluation{Spec: s, State: st}).Bool(x)
}

// ValidateExpr statically checks every operand, including branches unreachable in samples.
func ValidateExpr(s *model.Spec, x any, params map[string][]any, chain map[string]bool) error {
	if x == nil {
		return fmt.Errorf("expression is required")
	}
	m, ok := x.(map[string]any)
	if !ok {
		switch x.(type) {
		case string, bool, float64, int, json.Number:
			return nil
		}
		return fmt.Errorf("invalid literal")
	}
	if len(m) != 1 {
		return fmt.Errorf("expression needs one operator")
	}
	for op, arg := range m {
		switch op {
		case "var":
			k, ok := arg.(string)
			if !ok {
				return fmt.Errorf("var must be a string")
			}
			if k != "$state" {
				if _, ok := s.Variables[k]; !ok {
					return fmt.Errorf("unknown variable %s", k)
				}
			}
		case "param":
			k, ok := arg.(string)
			if !ok {
				return fmt.Errorf("param must be a string")
			}
			if _, ok := params[k]; !ok {
				return fmt.Errorf("unknown/unavailable parameter %s", k)
			}
		case "condition":
			k, ok := arg.(string)
			if !ok {
				return fmt.Errorf("condition must be a string")
			}
			c, ok := s.Conditions[k]
			if !ok {
				return fmt.Errorf("unknown condition %s", k)
			}
			if chain == nil {
				chain = map[string]bool{}
			}
			if chain[k] {
				return fmt.Errorf("cyclic condition %s", k)
			}
			chain[k] = true
			err := ValidateExpr(s, c, params, chain)
			delete(chain, k)
			return err
		case "not":
			return ValidateExpr(s, arg, params, chain)
		case "and", "or", "eq", "ne", "lt", "le", "gt", "ge":
			a, ok := arg.([]any)
			if !ok || len(a) == 0 {
				return fmt.Errorf("%s needs operands", op)
			}
			if op != "and" && op != "or" && len(a) != 2 {
				return fmt.Errorf("%s needs 2 operands", op)
			}
			for _, v := range a {
				if err := ValidateExpr(s, v, params, chain); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unsupported operator %s", op)
		}
	}
	return nil
}
