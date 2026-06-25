package sumup

import (
	"testing"
	"time"
)

func TestIntervalNext(t *testing.T) {
	base := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		interval Interval
		want     time.Time
	}{
		"monthly":            {Monthly, base.AddDate(0, 1, 0)},
		"yearly":             {Yearly, base.AddDate(1, 0, 0)},
		"unknown is monthly": {Interval("weekly"), base.AddDate(0, 1, 0)},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tc.interval.Next(base); !got.Equal(tc.want) {
				t.Errorf("Next = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestChargeResultSuccessful(t *testing.T) {
	if !(ChargeResult{Status: StatusSuccessful}).Successful() {
		t.Error("StatusSuccessful should be Successful")
	}
	if (ChargeResult{Status: "FAILED"}).Successful() {
		t.Error("FAILED should not be Successful")
	}
}
