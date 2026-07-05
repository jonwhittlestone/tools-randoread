package handlers

import (
	"testing"
	"time"
)

func TestCurrentPeriodStartBeforeResetHour(t *testing.T) {
	// 10:00 BST on 2026-07-05 is before the 16:00 reset, so the period
	// started at 16:00 the previous day.
	now := time.Date(2026, 7, 5, 10, 0, 0, 0, randoLocation)
	want := time.Date(2026, 7, 4, 16, 0, 0, 0, randoLocation)

	if got := currentPeriodStart(now); !got.Equal(want) {
		t.Errorf("currentPeriodStart(%v) = %v, want %v", now, got, want)
	}
}

func TestCurrentPeriodStartAfterResetHour(t *testing.T) {
	now := time.Date(2026, 7, 5, 18, 0, 0, 0, randoLocation)
	want := time.Date(2026, 7, 5, 16, 0, 0, 0, randoLocation)

	if got := currentPeriodStart(now); !got.Equal(want) {
		t.Errorf("currentPeriodStart(%v) = %v, want %v", now, got, want)
	}
}

func TestCurrentPeriodStartAtExactResetHour(t *testing.T) {
	now := time.Date(2026, 7, 5, 16, 0, 0, 0, randoLocation)
	want := time.Date(2026, 7, 5, 16, 0, 0, 0, randoLocation)

	if got := currentPeriodStart(now); !got.Equal(want) {
		t.Errorf("currentPeriodStart(%v) = %v, want %v", now, got, want)
	}
}

func TestCurrentPeriodStartConvertsFromUTC(t *testing.T) {
	// 2026-07-05 15:00 UTC is 16:00 BST (UTC+1 in summer) — right at the
	// reset boundary — regardless of what timezone `now` is expressed in.
	now := time.Date(2026, 7, 5, 15, 0, 0, 0, time.UTC)
	want := time.Date(2026, 7, 5, 16, 0, 0, 0, randoLocation)

	if got := currentPeriodStart(now); !got.Equal(want) {
		t.Errorf("currentPeriodStart(%v) = %v, want %v", now, got, want)
	}
}
