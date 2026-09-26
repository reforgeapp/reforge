package heartbeat

import (
	"errors"
	"testing"
	"time"
)

func TestStatusFlagsStaleLoops(t *testing.T) {
	Beat("fresh", time.Second, errors.New("boom"))
	mu.Lock()
	loops["old"] = entry{at: time.Now().Add(-2 * time.Hour), every: time.Minute}
	mu.Unlock()
	status, healthy := Status()
	if healthy || len(status) != 2 || status[0].Name != "fresh" || status[0].LastError != "boom" || status[0].Stale || !status[1].Stale {
		t.Fatalf("status=%+v healthy=%v", status, healthy)
	}
}
