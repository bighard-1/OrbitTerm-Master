package workerstatus

import (
	"sync"
	"time"
)

type Status struct {
	Running       bool       `json:"running"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	FailureCount  uint64     `json:"failure_count"`
	LastFailed    bool       `json:"last_failed"`
}

var registry = struct {
	sync.Mutex
	items map[string]Status
}{items: make(map[string]Status)}

func Started(name string) {
	registry.Lock()
	defer registry.Unlock()
	s := registry.items[name]
	s.Running = true
	registry.items[name] = s
}
func Stopped(name string) {
	registry.Lock()
	defer registry.Unlock()
	s := registry.items[name]
	s.Running = false
	registry.items[name] = s
}
func Completed(name string, err error) {
	registry.Lock()
	defer registry.Unlock()
	s := registry.items[name]
	now := time.Now().UTC()
	s.LastRunAt = &now
	s.LastFailed = err != nil
	if err != nil {
		s.FailureCount++
	} else {
		s.LastSuccessAt = &now
	}
	registry.items[name] = s
}
func Snapshot() map[string]Status {
	registry.Lock()
	defer registry.Unlock()
	copy := make(map[string]Status, len(registry.items))
	for key, value := range registry.items {
		copy[key] = value
	}
	return copy
}
