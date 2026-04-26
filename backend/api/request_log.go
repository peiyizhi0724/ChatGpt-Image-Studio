package api

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const maxImageRequestLogEntries = 200

type imageRequestLogEntry struct {
	ID                    string `json:"id"`
	StartedAt             string `json:"startedAt"`
	FinishedAt            string `json:"finishedAt"`
	Endpoint              string `json:"endpoint"`
	Operation             string `json:"operation"`
	ImageMode             string `json:"imageMode"`
	Direction             string `json:"direction"`
	Route                 string `json:"route"`
	CPASubroute           string `json:"cpaSubroute,omitempty"`
	QueueWaitMS           int64  `json:"queueWaitMs,omitempty"`
	InflightCountAtStart  int    `json:"inflightCountAtStart,omitempty"`
	LeaseAcquired         bool   `json:"leaseAcquired,omitempty"`
	ErrorCode             string `json:"errorCode,omitempty"`
	RoutingPolicyApplied  bool   `json:"routingPolicyApplied,omitempty"`
	RoutingGroupIndex     int    `json:"routingGroupIndex,omitempty"`
	RoutingSortMode       string `json:"routingSortMode,omitempty"`
	RoutingReservePercent int    `json:"routingReservePercent,omitempty"`
	AccountType           string `json:"accountType,omitempty"`
	AccountEmail          string `json:"accountEmail,omitempty"`
	AccountFile           string `json:"accountFile,omitempty"`
	RequestedModel        string `json:"requestedModel,omitempty"`
	UpstreamModel         string `json:"upstreamModel,omitempty"`
	ImageToolModel        string `json:"imageToolModel,omitempty"`
	Size                  string `json:"size,omitempty"`
	Quality               string `json:"quality,omitempty"`
	PromptLength          int    `json:"promptLength,omitempty"`
	Preferred             bool   `json:"preferred"`
	Success               bool   `json:"success"`
	Error                 string `json:"error,omitempty"`
}

type imageRequestLogStore struct {
	mu    sync.Mutex
	items []imageRequestLogEntry
}

func newImageRequestLogStore() *imageRequestLogStore {
	return &imageRequestLogStore{
		items: make([]imageRequestLogEntry, 0, maxImageRequestLogEntries),
	}
}

func (s *imageRequestLogStore) add(entry imageRequestLogEntry) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(entry.ID) == "" {
		entry.ID = fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	s.items = append([]imageRequestLogEntry{entry}, s.items...)
	if len(s.items) > maxImageRequestLogEntries {
		s.items = s.items[:maxImageRequestLogEntries]
	}
}

func (s *imageRequestLogStore) list(limit int) []imageRequestLogEntry {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 || limit > len(s.items) {
		limit = len(s.items)
	}
	out := make([]imageRequestLogEntry, limit)
	copy(out, s.items[:limit])
	return out
}
