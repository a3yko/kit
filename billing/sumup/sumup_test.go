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
		"weekly":             {Weekly, base.AddDate(0, 0, 7)},
		"monthly":            {Monthly, base.AddDate(0, 1, 0)},
		"quarterly":          {Quarterly, base.AddDate(0, 3, 0)},
		"semi_annually":      {SemiAnnually, base.AddDate(0, 6, 0)},
		"yearly":             {Yearly, base.AddDate(1, 0, 0)},
		"unknown is monthly": {Interval("fortnightly"), base.AddDate(0, 1, 0)},
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
