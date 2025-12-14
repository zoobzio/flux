package flux

import "sync"

// errorRing is a thread-safe ring buffer for storing recent errors.
type errorRing struct {
	mu     sync.RWMutex
	errors []error
	size   int
	head   int
	count  int
}

// newErrorRing creates a new error ring buffer with the given capacity.
// If size is 0, the ring buffer is disabled.
func newErrorRing(size int) *errorRing {
	if size <= 0 {
		return nil
	}
	return &errorRing{
		errors: make([]error, size),
		size:   size,
	}
}

// push adds an error to the ring buffer.
func (r *errorRing) push(err error) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.errors[r.head] = err
	r.head = (r.head + 1) % r.size
	if r.count < r.size {
		r.count++
	}
}

// clear removes all errors from the ring buffer.
func (r *errorRing) clear() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.errors {
		r.errors[i] = nil
	}
	r.head = 0
	r.count = 0
}

// all returns all errors in the ring buffer, oldest first.
func (r *errorRing) all() []error {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.count == 0 {
		return nil
	}

	result := make([]error, r.count)
	start := (r.head - r.count + r.size) % r.size
	for i := 0; i < r.count; i++ {
		result[i] = r.errors[(start+i)%r.size]
	}
	return result
}
