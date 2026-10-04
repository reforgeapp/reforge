package runnerclient

import (
	"context"
	"testing"
	"time"
)

func TestDrainingOutlivesStopUntilTimeout(t *testing.T) {
	stop, signal := context.WithCancel(context.Background())
	work, cancel := Draining(stop, 50*time.Millisecond)
	defer cancel()
	signal()
	select {
	case <-work.Done():
		t.Fatal("running work cancelled at the stop signal")
	case <-time.After(20 * time.Millisecond):
	}
	select {
	case <-work.Done():
	case <-time.After(time.Second):
		t.Fatal("running work outlived the drain timeout")
	}
}
