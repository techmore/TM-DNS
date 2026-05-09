package systemstats

import "testing"

func TestParsePMSetCustomDetectsSleep(t *testing.T) {
	power := parsePMSetCustom(`Battery Power:
 sleep                10
 displaysleep         5
AC Power:
 sleep                0
 displaysleep         10
`)
	if !power.Supported {
		t.Fatal("expected supported")
	}
	if !power.SleepConfigured {
		t.Fatal("expected sleep warning")
	}
	if power.SystemSleepMinutes != 10 {
		t.Fatalf("sleep minutes = %d", power.SystemSleepMinutes)
	}
	if power.Profile != "Battery Power" {
		t.Fatalf("profile = %q", power.Profile)
	}
}

func TestParsePMSetCustomAllowsDisabledSleep(t *testing.T) {
	power := parsePMSetCustom(`AC Power:
 sleep                0
 displaysleep         10
`)
	if power.SleepConfigured {
		t.Fatal("did not expect sleep warning")
	}
	if power.Status != "ok" {
		t.Fatalf("status = %q", power.Status)
	}
}
