package audit

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

type AuditEvent struct {
	Timestamp  int64          `json:"timestamp"`
	RequestID  string         `json:"requestId"`
	TenantID   string         `json:"tenantId"`
	SourceID   string         `json:"sourceId"`
	FlowID     string         `json:"flowId,omitempty"`
	Step       string         `json:"step"`
	Outcome    string         `json:"outcome"`
	ReasonCode string         `json:"reason_code,omitempty"`
	LatencyMs  int64          `json:"latency_ms"`
	Metrics    map[string]any `json:"metrics,omitempty"`
}

type Logger interface {
	Emit(event AuditEvent)
	GetRecent() []AuditEvent
}

type RingBufferLogger struct {
	mu     sync.RWMutex
	events []AuditEvent
	head   int
	count  int
	size   int
}

func NewRingBufferLogger(size int) *RingBufferLogger {
	return &RingBufferLogger{
		events: make([]AuditEvent, size),
		size:   size,
	}
}

func (r *RingBufferLogger) Emit(event AuditEvent) {
	// Enforce metadata-only rules here or logically prior.
	// We never log inputs or outputs.
	if event.Timestamp == 0 {
		event.Timestamp = time.Now().UnixMilli()
	}

	rawJSON, err := json.Marshal(event)
	if err == nil {
		log.Printf("AUDIT %s", string(rawJSON))
	} else {
		log.Printf("AUDIT ERROR marshalling event: %v", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.events[r.head] = event
	r.head = (r.head + 1) % r.size
	if r.count < r.size {
		r.count++
	}
}

func (r *RingBufferLogger) GetRecent() []AuditEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]AuditEvent, 0, r.count)
	idx := r.head
	for i := 0; i < r.count; i++ {
		idx = (idx - 1 + r.size) % r.size
		result = append(result, r.events[idx])
	}
	
	// Reverse to get chronological if desired, or leave reverse-chronological so index 0 is newest.
	// Currently returning reverse-chronological.
	return result
}
