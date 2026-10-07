package http

import (
	"sync"
	"testing"
)

func TestPasswordFailureCountersAreSafeBeforeServerStart(t *testing.T) {
	// The counters must be usable before Start launches the hourly cleanup
	// goroutines; this was the nil-map panic seen by an early request.
	clearCheckPasswordFailures()
	clearPasswordUpdateFailures()
	incrementCheckPasswordFailure("alice")
	incrementPasswordUpdateFailure("alice")
	if got := checkPasswordFailureCount("alice"); got != 1 {
		t.Fatalf("check failure count = %d, want 1", got)
	}
	if got := passwordUpdateFailureCount("alice"); got != 1 {
		t.Fatalf("update failure count = %d, want 1", got)
	}
}

func TestPasswordFailureCountersConcurrent(t *testing.T) {
	clearCheckPasswordFailures()
	clearPasswordUpdateFailures()

	const workers = 32
	const increments = 100
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < increments; j++ {
				incrementCheckPasswordFailure("alice")
				incrementPasswordUpdateFailure("alice")
			}
		}()
	}
	wg.Wait()

	// Counters saturate at int8 max by design; the assertion verifies that all
	// concurrent writes completed without a race or map panic.
	if got := checkPasswordFailureCount("alice"); got != 127 {
		t.Fatalf("check failure count = %d, want saturation at 127", got)
	}
	if got := passwordUpdateFailureCount("alice"); got != 127 {
		t.Fatalf("update failure count = %d, want saturation at 127", got)
	}

	clearCheckPasswordFailures()
	clearPasswordUpdateFailures()
	if got := checkPasswordFailureCount("alice"); got != 0 {
		t.Fatalf("check failure count after clear = %d, want 0", got)
	}
	if got := passwordUpdateFailureCount("alice"); got != 0 {
		t.Fatalf("update failure count after clear = %d, want 0", got)
	}
}
