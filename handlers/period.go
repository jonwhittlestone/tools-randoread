package handlers

import (
	"log"
	"time"

	// Embeds the IANA tz database in the binary so Europe/London resolves
	// even on minimal container images that don't ship an OS tzdata package.
	_ "time/tzdata"
)

// randoResetHour is when Rando's daily pick rotates — 4pm, per Jon.
const randoResetHour = 16

var randoLocation *time.Location

func init() {
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		log.Fatalf("failed to load Europe/London timezone: %v", err)
	}
	randoLocation = loc
}

// currentPeriodStart returns the most recent randoResetHour timestamp at or
// before now, in Europe/London wall-clock time. Two calls with a "now"
// inside the same period return an identical value — that equality is what
// Rando uses to decide whether to keep serving today's pick or choose a new
// one.
func currentPeriodStart(now time.Time) time.Time {
	local := now.In(randoLocation)
	reset := time.Date(local.Year(), local.Month(), local.Day(), randoResetHour, 0, 0, 0, randoLocation)
	if local.Before(reset) {
		reset = reset.AddDate(0, 0, -1)
	}
	return reset
}
