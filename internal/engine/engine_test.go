package engine_test

import (
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/engine"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/testkit"
)

// A composite with an inherited event, an overriding child event, a staying
// event, a decision with a staying branch, a chained decision and a composite
// whose start is a decision.
const machine = `{"schemaVersion":"1","start":{"to":"業務"},"actors":{"編集者":{},"閲覧者":{}},
 "states":{
  "業務":{"start":{"to":"一覧"},
   "events":{"設定":{"actors":["編集者","閲覧者"],"to":"設定"}},
   "states":{
    "一覧":{"events":{
      "更新":{"actors":["編集者","閲覧者"],"action":"再表示する"},
      "開く":{"actors":["編集者","閲覧者"],"decision":{"branches":{"あり":{"to":"詳細"},"なし":{"action":"知らせる"}}}}}},
    "詳細":{"events":{
      "保存":{"actors":["編集者"],"decision":{"branches":{"失敗":{},"成功":{"decision":{"branches":{"通知":{"to":"一覧"},"無通知":{"to":"詳細"}}}}}}},
      "設定":{"actors":["編集者"],"decision":{"branches":{"破棄":{"to":"設定"},"取消":{}}}}}}}},
  "設定":{"start":{"decision":{"label":"初回","branches":{"初回":{"to":"案内"},"再訪":{"to":"通常"}}}},
   "states":{"案内":{},"通常":{}}}}}`

func setup(t *testing.T) *engine.Engine {
	t.Helper()
	return engine.New(testkit.Compile(t, machine))
}

func TestInheritedAndOverridingEvents(t *testing.T) {
	e := setup(t)
	if got := e.Available("業務/一覧")["設定"]; got != "業務:設定" {
		t.Fatalf("parent event not inherited: %s", got)
	}
	if got := e.Available("業務/詳細")["設定"]; got != "業務/詳細:設定" {
		t.Fatalf("child event does not override: %s", got)
	}
	v, path, err := e.Play(model.Snapshot{State: "業務/詳細"}, model.Step{Event: "設定", Branches: []string{"取消"}}, "編集者")
	if err != nil || v.State != "業務/詳細" || len(path) != 2 {
		t.Fatalf("overriding decision: %+v %v %v", v, path, err)
	}
}

func TestStayingEventAndBranchReturnToTheRaisingLeaf(t *testing.T) {
	e := setup(t)
	v, path, err := e.Play(model.Snapshot{State: "業務/一覧"}, model.Step{Event: "更新"}, "閲覧者")
	if err != nil || v.State != "業務/一覧" || strings.Join(path, ",") != "業務/一覧:更新" {
		t.Fatalf("staying event: %+v %v %v", v, path, err)
	}
	v, path, err = e.Play(model.Snapshot{State: "業務/一覧"}, model.Step{Event: "開く", Branches: []string{"なし"}}, "閲覧者")
	if err != nil || v.State != "業務/一覧" || strings.Join(path, ",") != "業務/一覧:開く,業務/一覧:開く>なし" {
		t.Fatalf("staying branch: %+v %v %v", v, path, err)
	}
}

func TestChainedDecisionsAndStartDecisions(t *testing.T) {
	e := setup(t)
	v, path, err := e.Play(model.Snapshot{State: "業務/詳細"}, model.Step{Event: "保存", Branches: []string{"成功", "通知"}}, "編集者")
	if err != nil || v.State != "業務/一覧" || len(path) != 3 || path[2] != "業務/詳細:保存>成功>通知" {
		t.Fatalf("chained decision: %+v %v %v", v, path, err)
	}
	v, path, err = e.Play(model.Snapshot{State: "業務/一覧"}, model.Step{Event: "設定", Branches: []string{"再訪"}}, "閲覧者")
	if err != nil || v.State != "設定/通常" || strings.Join(path, ",") != "業務:設定,設定:[開始]>再訪" {
		t.Fatalf("entering a composite through its start decision: %+v %v %v", v, path, err)
	}
	if start, err := e.Start("設定"); err != nil || start.State != "設定:[開始]?" {
		t.Fatalf("start decision is not reported as unresolved: %+v %v", start, err)
	}
}

func TestPlayErrors(t *testing.T) {
	e := setup(t)
	at := model.Snapshot{State: "業務/詳細"}
	for name, tc := range map[string]struct {
		step  model.Step
		actor string
		want  string
	}{
		"missing branch":  {model.Step{Event: "保存"}, "編集者", "needs a branch"},
		"unknown branch":  {model.Step{Event: "保存", Branches: []string{"保留"}}, "編集者", "has no branch"},
		"extra branch":    {model.Step{Event: "保存", Branches: []string{"失敗", "通知"}}, "編集者", "passes no decision"},
		"unknown event":   {model.Step{Event: "削除"}, "編集者", "not available"},
		"actor":           {model.Step{Event: "保存", Branches: []string{"失敗"}}, "閲覧者", "cannot raise"},
		"rejected but ok": {model.Step{Event: "保存", Reject: true}, "編集者", "expected to be rejected"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := e.Play(at, tc.step, tc.actor); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
	if v, path, err := e.Play(at, model.Step{Event: "保存", Reject: true}, "閲覧者"); err != nil || v != at || path != nil {
		t.Fatalf("permission rejection: %+v %v %v", v, path, err)
	}
}

func TestReplayUseCaseRules(t *testing.T) {
	e := setup(t)
	u := model.UseCase{Actor: "編集者", Starts: []string{"業務/詳細"}, Ends: []string{"業務/詳細:保存>成功"}, Scope: []string{"業務/詳細:保存"}}
	scenario := func(steps ...model.Step) model.Scenario {
		return model.Scenario{Name: "s", Goal: "g", ExpectedOutcome: "o", Start: "業務/詳細", Steps: steps}
	}
	ok := e.Replay("u/ok", scenario(model.Step{Event: "保存", Branches: []string{"失敗"}}, model.Step{Event: "保存", Branches: []string{"成功", "無通知"}}), &u)
	if !ok.Valid || len(ok.Witness.Transitions) != 5 || len(ok.Trace) != 3 || ok.Actor != "編集者" {
		t.Fatalf("valid scenario: %+v", ok)
	}
	for name, tc := range map[string]struct {
		sc   model.Scenario
		want string
	}{
		"not ended":    {scenario(model.Step{Event: "保存", Branches: []string{"失敗"}}), "did not reach a use-case end"},
		"past end":     {scenario(model.Step{Event: "保存", Branches: []string{"成功", "無通知"}}, model.Step{Event: "保存", Branches: []string{"失敗"}}), "continues past use-case end"},
		"out of scope": {scenario(model.Step{Event: "設定", Branches: []string{"取消"}}), "outside the use-case scope"},
		"reject":       {scenario(model.Step{Event: "設定", Reject: true}), "reject steps belong in regressions"},
		"wrong start":  {model.Scenario{Start: "業務/一覧", Steps: []model.Step{{Event: "更新"}}}, "not a declared use-case start"},
		"invalid step": {scenario(model.Step{Event: "保存"}), "step 1"},
	} {
		t.Run(name, func(t *testing.T) {
			if r := e.Replay("u/x", tc.sc, &u); r.Valid || !strings.Contains(r.Error, tc.want) {
				t.Fatalf("want %q, got %+v", tc.want, r)
			}
		})
	}
	regression := model.Scenario{Actor: "閲覧者", Start: "業務/詳細", Steps: []model.Step{{Event: "保存", Reject: true}}}
	if r := e.Replay("regressions/r", regression, nil); !r.Valid {
		t.Fatalf("regression with a permission rejection: %+v", r)
	}
}
