package build

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

// Release frees a slot.
func (s *Semaphore) Release() {
	<-s.ch
}
