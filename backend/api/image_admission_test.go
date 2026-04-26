package api

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestImageConcurrencyGateReleasesSlots(t *testing.T) {
	gate := newImageConcurrencyGate(2, 4, 2*time.Second)

	_, releaseOne, err := gate.acquire(context.Background())
	if err != nil {
		t.Fatalf("first acquire error: %v", err)
	}
	defer releaseOne()

	_, releaseTwo, err := gate.acquire(context.Background())
	if err != nil {
		t.Fatalf("second acquire error: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, releaseThree, acquireErr := gate.acquire(context.Background())
		if acquireErr == nil && releaseThree != nil {
			releaseThree()
		}
		done <- acquireErr
	}()

	select {
	case err := <-done:
		t.Fatalf("third acquire unexpectedly completed early: %v", err)
	case <-time.After(80 * time.Millisecond):
	}

	releaseTwo()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("third acquire failed after release: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("third acquire did not proceed after releasing slot")
	}
}

func TestImageConcurrencyGateQueueFull(t *testing.T) {
	gate := newImageConcurrencyGate(1, 1, 2*time.Second)

	_, releaseActive, err := gate.acquire(context.Background())
	if err != nil {
		t.Fatalf("active acquire error: %v", err)
	}
	defer releaseActive()

	blockedDone := make(chan struct{})
	go func() {
		_, releaseBlocked, acquireErr := gate.acquire(context.Background())
		if acquireErr == nil && releaseBlocked != nil {
			releaseBlocked()
		}
		close(blockedDone)
	}()

	select {
	case <-blockedDone:
		t.Fatal("expected second request to remain queued")
	case <-time.After(80 * time.Millisecond):
	}

	_, _, err = gate.acquire(context.Background())
	if !errors.Is(err, errImageAdmissionQueueFull) {
		t.Fatalf("third acquire error = %v, want errImageAdmissionQueueFull", err)
	}
}

func TestImageConcurrencyGateQueueTimeout(t *testing.T) {
	gate := newImageConcurrencyGate(1, 1, 60*time.Millisecond)

	_, releaseActive, err := gate.acquire(context.Background())
	if err != nil {
		t.Fatalf("active acquire error: %v", err)
	}
	defer releaseActive()

	start := time.Now()
	_, _, err = gate.acquire(context.Background())
	if !errors.Is(err, errImageAdmissionQueueTimeout) {
		t.Fatalf("queued acquire error = %v, want errImageAdmissionQueueTimeout", err)
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Fatalf("queue timeout elapsed too short: %s", elapsed)
	}
}
