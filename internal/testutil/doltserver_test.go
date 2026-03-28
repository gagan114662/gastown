//go:build !windows

package testutil

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestIsDockerAvailable_UsesProbeResult(t *testing.T) {
	oldOnce := dockerOnce
	oldAvail := dockerAvail
	oldTimeout := dockerProbeTimeout
	oldProbe := dockerAvailabilityFunc
	t.Cleanup(func() {
		dockerOnce = oldOnce
		dockerAvail = oldAvail
		dockerProbeTimeout = oldTimeout
		dockerAvailabilityFunc = oldProbe
	})

	dockerOnce = sync.Once{}
	dockerAvail = false
	dockerProbeTimeout = 50 * time.Millisecond
	dockerAvailabilityFunc = func(ctx context.Context) error {
		return nil
	}

	if !isDockerAvailable() {
		t.Fatal("expected Docker probe success to report available")
	}
}

func TestIsDockerAvailable_TimesOutAndCachesFailure(t *testing.T) {
	oldOnce := dockerOnce
	oldAvail := dockerAvail
	oldTimeout := dockerProbeTimeout
	oldProbe := dockerAvailabilityFunc
	t.Cleanup(func() {
		dockerOnce = oldOnce
		dockerAvail = oldAvail
		dockerProbeTimeout = oldTimeout
		dockerAvailabilityFunc = oldProbe
	})

	var calls int
	dockerOnce = sync.Once{}
	dockerAvail = false
	dockerProbeTimeout = 20 * time.Millisecond
	dockerAvailabilityFunc = func(ctx context.Context) error {
		calls++
		<-ctx.Done()
		return ctx.Err()
	}

	start := time.Now()
	if isDockerAvailable() {
		t.Fatal("expected timed-out Docker probe to report unavailable")
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("Docker probe took too long: %v", elapsed)
	}
	if calls != 1 {
		t.Fatalf("probe calls = %d, want 1", calls)
	}

	// The cached result should avoid invoking the probe again.
	dockerAvailabilityFunc = func(ctx context.Context) error {
		return errors.New("should not be called")
	}
	if isDockerAvailable() {
		t.Fatal("expected cached unavailable result to remain false")
	}
	if calls != 1 {
		t.Fatalf("probe calls after cache = %d, want 1", calls)
	}
}
