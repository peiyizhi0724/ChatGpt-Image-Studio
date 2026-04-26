package api

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	errImageAdmissionQueueFull    = errors.New("image admission queue full")
	errImageAdmissionQueueTimeout = errors.New("image admission queue timeout")
)

type imageAdmissionInfo struct {
	QueueWaitMS          int64
	InflightCountAtStart int
}

type imageAdmissionController struct {
	mu        sync.Mutex
	configKey string
	gate      *imageConcurrencyGate
}

type imageAdmissionSnapshot struct {
	MaxConcurrency int
	QueueLimit     int
	QueueTimeoutMS int64
	Inflight       int
	Queued         int
}

func newImageAdmissionController() *imageAdmissionController {
	return &imageAdmissionController{}
}

func (c *imageAdmissionController) acquire(ctx context.Context, maxConcurrent, queueLimit int, queueTimeout time.Duration) (imageAdmissionInfo, func(), error) {
	if maxConcurrent <= 0 {
		return imageAdmissionInfo{}, func() {}, nil
	}
	if queueLimit <= 0 {
		queueLimit = maxConcurrent
	}
	if queueTimeout <= 0 {
		queueTimeout = 20 * time.Second
	}

	key := fmt.Sprintf("%d:%d:%d", maxConcurrent, queueLimit, int(queueTimeout/time.Millisecond))

	c.mu.Lock()
	if c.gate == nil || c.configKey != key {
		c.gate = newImageConcurrencyGate(maxConcurrent, queueLimit, queueTimeout)
		c.configKey = key
	}
	gate := c.gate
	c.mu.Unlock()

	return gate.acquire(ctx)
}

func (c *imageAdmissionController) snapshot(maxConcurrent, queueLimit int, queueTimeout time.Duration) imageAdmissionSnapshot {
	if maxConcurrent <= 0 {
		return imageAdmissionSnapshot{}
	}
	if queueLimit <= 0 {
		queueLimit = maxConcurrent
	}
	if queueTimeout <= 0 {
		queueTimeout = 20 * time.Second
	}

	key := fmt.Sprintf("%d:%d:%d", maxConcurrent, queueLimit, int(queueTimeout/time.Millisecond))

	c.mu.Lock()
	if c.gate == nil || c.configKey != key {
		c.gate = newImageConcurrencyGate(maxConcurrent, queueLimit, queueTimeout)
		c.configKey = key
	}
	gate := c.gate
	c.mu.Unlock()

	return gate.snapshot()
}

type imageConcurrencyGate struct {
	slots        chan struct{}
	queue        chan struct{}
	queueTimeout time.Duration
}

func newImageConcurrencyGate(maxConcurrent, queueLimit int, queueTimeout time.Duration) *imageConcurrencyGate {
	return &imageConcurrencyGate{
		slots:        make(chan struct{}, maxConcurrent),
		queue:        make(chan struct{}, queueLimit),
		queueTimeout: queueTimeout,
	}
}

func (g *imageConcurrencyGate) acquire(ctx context.Context) (imageAdmissionInfo, func(), error) {
	info := imageAdmissionInfo{}
	if g == nil {
		return info, func() {}, nil
	}

	select {
	case g.queue <- struct{}{}:
	default:
		info.InflightCountAtStart = len(g.slots)
		return info, nil, errImageAdmissionQueueFull
	}

	enqueuedAt := time.Now()
	queueHeld := true
	releaseQueue := func() {
		if !queueHeld {
			return
		}
		<-g.queue
		queueHeld = false
	}

	timer := time.NewTimer(g.queueTimeout)
	defer timer.Stop()

	select {
	case g.slots <- struct{}{}:
		releaseQueue()
		info.QueueWaitMS = time.Since(enqueuedAt).Milliseconds()
		inflight := len(g.slots) - 1
		if inflight < 0 {
			inflight = 0
		}
		info.InflightCountAtStart = inflight
		var once sync.Once
		release := func() {
			once.Do(func() {
				<-g.slots
			})
		}
		return info, release, nil
	case <-ctx.Done():
		releaseQueue()
		info.QueueWaitMS = time.Since(enqueuedAt).Milliseconds()
		info.InflightCountAtStart = len(g.slots)
		return info, nil, ctx.Err()
	case <-timer.C:
		releaseQueue()
		info.QueueWaitMS = time.Since(enqueuedAt).Milliseconds()
		info.InflightCountAtStart = len(g.slots)
		return info, nil, errImageAdmissionQueueTimeout
	}
}

func (g *imageConcurrencyGate) snapshot() imageAdmissionSnapshot {
	if g == nil {
		return imageAdmissionSnapshot{}
	}
	return imageAdmissionSnapshot{
		MaxConcurrency: cap(g.slots),
		QueueLimit:     cap(g.queue),
		QueueTimeoutMS: g.queueTimeout.Milliseconds(),
		Inflight:       len(g.slots),
		Queued:         len(g.queue),
	}
}

func (s *Server) acquireImageAdmission(ctx context.Context) (imageAdmissionInfo, func(), error) {
	if s == nil || s.cfg == nil || s.imageAdmission == nil {
		return imageAdmissionInfo{}, func() {}, nil
	}
	maxConcurrent, queueLimit, queueTimeout := s.cfg.ImageQueueConfig()
	return s.imageAdmission.acquire(ctx, maxConcurrent, queueLimit, queueTimeout)
}

type imageAdmissionContextKey struct{}

func withImageAdmissionInfo(ctx context.Context, info imageAdmissionInfo) context.Context {
	return context.WithValue(ctx, imageAdmissionContextKey{}, info)
}

func imageAdmissionFromContext(ctx context.Context) imageAdmissionInfo {
	if ctx == nil {
		return imageAdmissionInfo{}
	}
	info, ok := ctx.Value(imageAdmissionContextKey{}).(imageAdmissionInfo)
	if !ok {
		return imageAdmissionInfo{}
	}
	return info
}
