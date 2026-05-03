package build

import "context"

// Semaphore limits concurrent builds.
type Semaphore struct {
	ch chan struct{}
}

// NewSemaphore creates a semaphore with the given max concurrency.
func NewSemaphore(max int) *Semaphore {
	if max <= 0 {
		max = 1
	}
	return &Semaphore{ch: make(chan struct{}, max)}
}

// Acquire blocks until a slot is available.
func (s *Semaphore) Acquire() {
	s.ch <- struct{}{}
}

// AcquireCtx blocks until a slot is available or ctx is done.
// Returns ctx.Err() if the context is canceled before a slot opens.
func (s *Semaphore) AcquireCtx(ctx context.Context) error {
	select {
	case s.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release frees a slot.
func (s *Semaphore) Release() {
	<-s.ch
}
