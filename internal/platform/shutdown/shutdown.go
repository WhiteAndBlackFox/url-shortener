// Package shutdown provides a small helper for bounding otherwise-unbounded
// blocking shutdown calls (grpc.Server.GracefulStop, sync.WaitGroup.Wait,
// ...), so a stuck dependency can't hang a process past the container
// orchestrator's SIGTERM-to-SIGKILL grace period.
package shutdown

import "time"

// WaitTimeout runs wait (expected to block until some shutdown step
// completes) and returns true if it did so within timeout, or false if
// timeout elapsed first — in which case wait's goroutine is left running
// (it wasn't designed to be canceled), but the caller is free to proceed
// with a harder shutdown step instead of blocking forever.
func WaitTimeout(timeout time.Duration, wait func()) bool {
	done := make(chan struct{})
	go func() {
		wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}
