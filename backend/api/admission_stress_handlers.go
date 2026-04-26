package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"sync"
	"time"
)

type admissionStressRequest struct {
	Total          int `json:"total"`
	Workers        int `json:"workers"`
	HoldMS         int `json:"holdMs"`
	TimeoutSeconds int `json:"timeoutSeconds"`
}

type admissionStressResponse struct {
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
	DurationMS int64  `json:"durationMs"`
	TimedOut   bool   `json:"timedOut"`
	Input      struct {
		Total          int `json:"total"`
		Workers        int `json:"workers"`
		HoldMS         int `json:"holdMs"`
		TimeoutSeconds int `json:"timeoutSeconds"`
	} `json:"input"`
	Admission struct {
		MaxConcurrency int   `json:"maxConcurrency"`
		QueueLimit     int   `json:"queueLimit"`
		QueueTimeoutMS int64 `json:"queueTimeoutMs"`
	} `json:"admission"`
	Counters struct {
		Submitted    int `json:"submitted"`
		Attempted    int `json:"attempted"`
		Admitted     int `json:"admitted"`
		QueueFull    int `json:"queueFull"`
		QueueTimeout int `json:"queueTimeout"`
		Canceled     int `json:"canceled"`
		OtherErrors  int `json:"otherErrors"`
	} `json:"counters"`
	QueueWait struct {
		Samples int     `json:"samples"`
		AvgMS   float64 `json:"avgMs"`
		P50MS   int64   `json:"p50Ms"`
		P95MS   int64   `json:"p95Ms"`
		MaxMS   int64   `json:"maxMs"`
	} `json:"queueWait"`
}

type admissionStressMetrics struct {
	mu sync.Mutex

	attempted    int
	admitted     int
	queueFull    int
	queueTimeout int
	canceled     int
	otherErrors  int
	queueWaitMS  []int64
}

func (m *admissionStressMetrics) recordAttempt() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attempted++
}

func (m *admissionStressMetrics) recordAdmitted(waitMS int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.admitted++
	m.queueWaitMS = append(m.queueWaitMS, waitMS)
}

func (m *admissionStressMetrics) recordError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case errors.Is(err, errImageAdmissionQueueFull):
		m.queueFull++
	case errors.Is(err, errImageAdmissionQueueTimeout):
		m.queueTimeout++
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		m.canceled++
	default:
		m.otherErrors++
	}
}

func (m *admissionStressMetrics) snapshot() admissionStressMetrics {
	m.mu.Lock()
	defer m.mu.Unlock()
	return admissionStressMetrics{
		attempted:    m.attempted,
		admitted:     m.admitted,
		queueFull:    m.queueFull,
		queueTimeout: m.queueTimeout,
		canceled:     m.canceled,
		otherErrors:  m.otherErrors,
		queueWaitMS:  append([]int64(nil), m.queueWaitMS...),
	}
}

func (s *Server) handleAdmissionStress(w http.ResponseWriter, r *http.Request) {
	var body admissionStressRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	total := body.Total
	if total <= 0 {
		total = 120
	}
	if total > 5000 {
		total = 5000
	}

	workers := body.Workers
	if workers <= 0 {
		workers = 20
	}
	if workers > 200 {
		workers = 200
	}
	if workers > total {
		workers = total
	}
	if workers <= 0 {
		workers = 1
	}

	holdMS := body.HoldMS
	if holdMS < 0 {
		holdMS = 0
	}
	if holdMS == 0 {
		holdMS = 1200
	}
	if holdMS > 120000 {
		holdMS = 120000
	}

	timeoutSeconds := body.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 60
	}
	if timeoutSeconds > 900 {
		timeoutSeconds = 900
	}

	startedAt := time.Now()
	timeout := time.Duration(timeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	jobs := make(chan struct{}, total)
	for i := 0; i < total; i++ {
		jobs <- struct{}{}
	}
	close(jobs)

	holdDuration := time.Duration(holdMS) * time.Millisecond
	metrics := &admissionStressMetrics{
		queueWaitMS: make([]int64, 0, total),
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				if ctx.Err() != nil {
					return
				}

				metrics.recordAttempt()
				info, release, err := s.acquireImageAdmission(ctx)
				if err != nil {
					metrics.recordError(err)
					continue
				}

				metrics.recordAdmitted(info.QueueWaitMS)
				timer := time.NewTimer(holdDuration)
				select {
				case <-timer.C:
					release()
				case <-ctx.Done():
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					release()
					metrics.recordError(ctx.Err())
					return
				}
			}
		}()
	}
	wg.Wait()

	maxConcurrent, queueLimit, queueTimeout := s.cfg.ImageQueueConfig()
	finishedAt := time.Now()
	result := metrics.snapshot()

	waitStats := summarizeQueueWait(result.queueWaitMS)
	resp := admissionStressResponse{
		StartedAt:  startedAt.Format(time.RFC3339Nano),
		FinishedAt: finishedAt.Format(time.RFC3339Nano),
		DurationMS: finishedAt.Sub(startedAt).Milliseconds(),
		TimedOut:   errors.Is(ctx.Err(), context.DeadlineExceeded),
	}
	resp.Input.Total = total
	resp.Input.Workers = workers
	resp.Input.HoldMS = holdMS
	resp.Input.TimeoutSeconds = timeoutSeconds
	resp.Admission.MaxConcurrency = maxConcurrent
	resp.Admission.QueueLimit = queueLimit
	resp.Admission.QueueTimeoutMS = queueTimeout.Milliseconds()
	resp.Counters.Submitted = total
	resp.Counters.Attempted = result.attempted
	resp.Counters.Admitted = result.admitted
	resp.Counters.QueueFull = result.queueFull
	resp.Counters.QueueTimeout = result.queueTimeout
	resp.Counters.Canceled = result.canceled
	resp.Counters.OtherErrors = result.otherErrors
	resp.QueueWait.Samples = len(result.queueWaitMS)
	resp.QueueWait.AvgMS = waitStats.AvgMS
	resp.QueueWait.P50MS = waitStats.P50MS
	resp.QueueWait.P95MS = waitStats.P95MS
	resp.QueueWait.MaxMS = waitStats.MaxMS

	writeJSON(w, http.StatusOK, resp)
}

type queueWaitStats struct {
	AvgMS float64
	P50MS int64
	P95MS int64
	MaxMS int64
}

func summarizeQueueWait(samples []int64) queueWaitStats {
	if len(samples) == 0 {
		return queueWaitStats{}
	}
	sorted := append([]int64(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	var total int64
	for _, value := range sorted {
		total += value
	}

	return queueWaitStats{
		AvgMS: float64(total) / float64(len(sorted)),
		P50MS: percentileValue(sorted, 0.50),
		P95MS: percentileValue(sorted, 0.95),
		MaxMS: sorted[len(sorted)-1],
	}
}

func percentileValue(sorted []int64, ratio float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	if ratio <= 0 {
		return sorted[0]
	}
	if ratio >= 1 {
		return sorted[len(sorted)-1]
	}
	index := int(float64(len(sorted)-1) * ratio)
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}
