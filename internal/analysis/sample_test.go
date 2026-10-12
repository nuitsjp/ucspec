package analysis

import (
	"github.com/nuitsjp/ucspec/internal/engine"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/testkit"
	"testing"
)

func TestSample(t *testing.T) {
	s := testkit.Sample(t)
	r := Analyze(s, "test")
	for _, d := range r.Diagnostics {
		t.Log(d.Code, d.Subject, d.Message)
	}
	for _, u := range r.UseCases {
		for _, c := range u.Coverage {
			t.Log(u.ID, c.Kind, c.Covered, c.Total, c.Complete, "infeasible", len(c.Infeasible))
		}
	}
	for _, c := range r.Checks {
		if c.Status != "verified" {
			t.Errorf("check %s: %+v", c.ID, c)
		}
	}
	if r.Status != "verified" {
		t.Fatalf("sample status %s", r.Status)
	}
}

func TestSampleSearchUsesOneState(t *testing.T) {
	s := testkit.Sample(t)
	e := engine.New(s)
	st, err := s.Normalize(model.Snapshot{State: "search"})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Children("search")) != 0 {
		t.Fatal("search has substates")
	}
	if _, _, err := e.Apply(st, "openCustomer", map[string]any{"exists": true}); err == nil {
		t.Fatal("customer opened before search")
	}
	screen := s.States[st.State].Screen
	for i := 0; i < 2; i++ {
		st, _, err = e.Apply(st, "searchCustomers", nil)
		if err != nil || st.State != "searchResult" || !s.IsChoice(st.State) {
			t.Fatalf("search did not reach the search result choice: %+v, %v", st, err)
		}
		st, _, err = e.Apply(st, "searchFound", nil)
		if err != nil {
			t.Fatal(err)
		}
		if st.State != "search" || s.States[st.State].Screen != screen || !model.Equal(st.Values["searched"], true) {
			t.Fatalf("search changed search screen: %+v", st)
		}
		if !model.Equal(st.Values["searchAlert"], false) {
			t.Fatal("nonempty search displayed an alert")
		}
	}
	st, _, err = e.Apply(st, "openCustomer", map[string]any{"exists": true})
	if err != nil || st.State != "customerAvailability" || !s.IsChoice(st.State) {
		t.Fatalf("search did not reach the customer availability choice: %+v, %v", st, err)
	}
	st, _, err = e.Apply(st, "openView", nil)
	if err != nil || st.State != "view" || !model.Equal(st.Values["outcome"], "viewed") {
		t.Fatalf("customer view did not open: %+v, %v", st, err)
	}
	st, _, err = e.Apply(st, "backResults", nil)
	if err != nil || st.State != "search" || s.States[st.State].Screen != screen {
		t.Fatalf("return to search changed the screen: %+v, %v", st, err)
	}
	st, _, err = e.Apply(st, "openCustomer", map[string]any{"exists": false})
	if err != nil || st.State != "customerAvailability" || !s.IsChoice(st.State) {
		t.Fatalf("disappeared customer did not reach the customer availability choice: %+v, %v", st, err)
	}
	st, _, err = e.Apply(st, "openGone", nil)
	if err != nil || st.State != "search" || !model.Equal(st.Values["searched"], true) || !model.Equal(st.Values["searchAlert"], true) || !model.Equal(st.Values["outcome"], "empty") {
		t.Fatalf("disappeared customer did not return to alerted search: %+v, %v", st, err)
	}
	st, err = s.Normalize(model.Snapshot{State: "search", Values: map[string]any{"exists": false}})
	if err != nil {
		t.Fatal(err)
	}
	st, _, err = e.Apply(st, "searchCustomers", nil)
	if err != nil || st.State != "searchResult" {
		t.Fatalf("empty search did not reach the search result choice: %+v, %v", st, err)
	}
	st, _, err = e.Apply(st, "searchEmpty", nil)
	if err != nil || st.State != "search" || s.States[st.State].Screen != screen || !model.Equal(st.Values["outcome"], "empty") || !model.Equal(st.Values["searchAlert"], true) {
		t.Fatalf("empty result changed search screen: %+v, %v", st, err)
	}
	if _, _, err := e.Apply(st, "openCustomer", map[string]any{"exists": true}); err == nil {
		t.Fatal("customer opened from zero results")
	}
}

func TestSampleSettingsStayOnOneScreenAndPreserveCommittedValue(t *testing.T) {
	s := testkit.Sample(t)
	e := engine.New(s)
	st, err := s.Normalize(model.Snapshot{State: "search"})
	if err != nil {
		t.Fatal(err)
	}
	apply := func(id string, input map[string]any) {
		t.Helper()
		st, _, err = e.Apply(st, id, input)
		if err != nil {
			t.Fatal(id, err)
		}
	}
	apply("openSettings", map[string]any{"discard": true})
	settingsScreen := s.States[st.State].Screen
	if s.Transitions["updateNotification"].Kind != "internal" {
		t.Fatal("notification update must be an internal transition")
	}
	apply("updateNotification", map[string]any{"enabled": true})
	if st.State != "notificationSettings" || s.States[st.State].Screen != settingsScreen || !model.Equal(st.Values["notificationEnabled"], true) {
		t.Fatalf("update did not commit on the settings screen: %+v", st)
	}
	before := model.CloneSnapshot(st)
	apply("keepSettingsForSearch", map[string]any{"discard": false})
	if !model.Equal(st, before) {
		t.Fatalf("declining navigation changed settings: %+v", st)
	}
	apply("returnSearchFromSettings", map[string]any{"discard": true})
	if st.State != "search" || !model.Equal(st.Values["notificationEnabled"], true) {
		t.Fatalf("navigation lost the committed setting: %+v", st)
	}
}

func TestSampleUpdateAndDeleteHaveNoProgressStates(t *testing.T) {
	s := testkit.Sample(t)
	if len(s.Children("view")) != 0 || len(s.Children("edit")) != 0 || len(s.Children("notificationSettings")) != 0 {
		t.Fatal("editors must be leaf states")
	}
	for _, id := range []string{"viewing", "confirming", "editing", "saving", "deleting", "notificationEditing", "notificationSaving"} {
		if s.HasVertex(id) {
			t.Fatalf("progress or redundant state remains: %s", id)
		}
	}
	e := engine.New(s)
	before, err := s.Normalize(model.Snapshot{State: "edit"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.Apply(before, "updateCustomer", map[string]any{"valid": false, "exists": true}); err == nil {
		t.Fatal("invalid input executed an update")
	}
	checked, _, err := e.Apply(before, "updateCustomer", map[string]any{"valid": true, "exists": true})
	if err != nil || checked.State != "customerAvailability" {
		t.Fatalf("update did not reach the customer availability choice: %+v, %v", checked, err)
	}
	after, _, err := e.Apply(checked, "openView", nil)
	if err != nil || after.State != "view" || !model.Equal(after.Values["revision"], "updated") || !model.Equal(after.Values["outcome"], "updated") {
		t.Fatalf("update did not commit and return to view: %+v, %v", after, err)
	}
	gone, _, err := e.Apply(before, "updateCustomer", map[string]any{"valid": true, "exists": false})
	if err == nil {
		gone, _, err = e.Apply(gone, "openGone", nil)
	}
	if err != nil || gone.State != "search" || !model.Equal(gone.Values["searchAlert"], true) {
		t.Fatalf("update of a deleted customer did not return to search with an alert: %+v, %v", gone, err)
	}
	confirmation, err := s.Normalize(model.Snapshot{State: "view"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Transitions["requestDelete"].Event != "requestDelete" || s.Transitions["requestDelete"].To != "deleteConfirmation" || !s.IsChoice("deleteConfirmation") || s.Transitions["cancelDelete"].Action != "" {
		t.Fatal("delete must enter the confirmation choice by one event with no cancellation action")
	}
	declined, _, err := e.Apply(confirmation, "requestDelete", map[string]any{"confirmed": false})
	if err != nil || declined.State != "deleteConfirmation" {
		t.Fatalf("declining delete did not reach the confirmation choice: %+v, %v", declined, err)
	}
	if _, _, err := e.Apply(declined, "confirmDelete", nil); err == nil {
		t.Fatal("declining confirmation executed deletion")
	}
	cancelled, _, err := e.Apply(declined, "cancelDelete", nil)
	if err != nil || cancelled.State != "view" || !model.Equal(cancelled.Values["exists"], true) || !model.Equal(cancelled.Values["revision"], confirmation.Values["revision"]) {
		t.Fatalf("declining confirmation changed the customer: %+v, %v", cancelled, err)
	}
	accepted, _, err := e.Apply(confirmation, "requestDelete", map[string]any{"confirmed": true})
	if err != nil || accepted.State != "deleteConfirmation" {
		t.Fatalf("accepting delete did not reach the confirmation choice: %+v, %v", accepted, err)
	}
	if _, _, err := e.Apply(accepted, "cancelDelete", nil); err == nil {
		t.Fatal("accepting confirmation enabled cancellation")
	}
	deleted, _, err := e.Apply(accepted, "confirmDelete", nil)
	if err != nil || deleted.State != "search" || !model.Equal(deleted.Values["exists"], false) || !model.Equal(deleted.Values["outcome"], "deleted") {
		t.Fatalf("confirmation did not delete and return to search: %+v, %v", deleted, err)
	}
}
