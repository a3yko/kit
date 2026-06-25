package sumup

import (
	"context"
	"testing"
	"time"
)

type fakeCharger struct {
	fn func(ChargeRequest) (ChargeResult, error)
}

func (f fakeCharger) Charge(_ context.Context, r ChargeRequest) (ChargeResult, error) {
	return f.fn(r)
}

type fakeStore struct {
	due      []Subscription
	recorded int
	advanced map[string]time.Time
}

func (s *fakeStore) DueForRenewal(context.Context, time.Time) ([]Subscription, error) {
	return s.due, nil
}

func (s *fakeStore) RecordCharge(context.Context, Subscription, ChargeResult, error) error {
	s.recorded++
	return nil
}

func (s *fakeStore) Advance(_ context.Context, sub Subscription, next time.Time) error {
	if s.advanced == nil {
		s.advanced = map[string]time.Time{}
	}
	s.advanced[sub.ID] = next
	return nil
}

func TestProcessDue(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{due: []Subscription{
		{ID: "ok", CustomerID: "ok", Interval: Monthly, NextBillingAt: now},
		{ID: "fail", CustomerID: "fail", Interval: Monthly, NextBillingAt: now},
	}}
	charger := fakeCharger{fn: func(r ChargeRequest) (ChargeResult, error) {
		if r.CustomerID == "ok" {
			return ChargeResult{TransactionID: "tx1", Status: "SUCCESSFUL"}, nil
		}
		return ChargeResult{Status: "FAILED"}, nil
	}}

	succeeded, err := NewEngine(charger, store).ProcessDue(context.Background(), now)

	if succeeded != 1 {
		t.Errorf("succeeded = %d, want 1", succeeded)
	}
	if err == nil {
		t.Error("expected a joined error for the failed charge, got nil")
	}
	if store.recorded != 2 {
		t.Errorf("recorded = %d, want 2 (both attempts recorded)", store.recorded)
	}
	if got, want := store.advanced["ok"], now.AddDate(0, 1, 0); !got.Equal(want) {
		t.Errorf("ok advanced to %v, want %v", got, want)
	}
	if _, advanced := store.advanced["fail"]; advanced {
		t.Error("failed subscription must not advance its billing date")
	}
}
