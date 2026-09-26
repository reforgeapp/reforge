package heartbeat

import (
	"sort"
	"sync"
	"time"
)

type Loop struct {
	Name      string    `json:"name"`
	LastCycle time.Time `json:"last_cycle"`
	LastError string    `json:"last_error,omitempty"`
	Stale     bool      `json:"stale"`
}

type entry struct {
	at    time.Time
	every time.Duration
	err   string
}

var (
	mu    sync.Mutex
	loops = map[string]entry{}
)

func Beat(name string, every time.Duration, err error) {
	e := entry{at: time.Now(), every: every}
	if err != nil {
		e.err = err.Error()
	}
	mu.Lock()
	loops[name] = e
	mu.Unlock()
}

func Status() ([]Loop, bool) {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Loop, 0, len(loops))
	healthy := true
	for name, e := range loops {
		stale := time.Since(e.at) > 3*e.every+time.Minute
		healthy = healthy && !stale
		out = append(out, Loop{Name: name, LastCycle: e.at.UTC(), LastError: e.err, Stale: stale})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, healthy
}
