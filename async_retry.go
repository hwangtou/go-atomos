package atomos

import (
	"sync"
	"time"
)

type asyncRetryRequest struct {
	target    string
	messageID string
	deliver   func(attempt int) error
	terminal  func(error)
	attempt   int
}

type asyncRetryQueue struct {
	requests  []*asyncRetryRequest
	running   bool
	executing bool
	timer     Timer
}

type asyncRetryManager struct {
	clock       Clock
	mutex       sync.Mutex
	queues      map[string]*asyncRetryQueue
	closed      bool
	maxAttempts int
	maxQueue    int
	shouldRetry func(error) bool
	backoff     func(attempt int) time.Duration
}

func newAsyncRetryManager(clock Clock, shouldRetry func(error) bool, backoff func(attempt int) time.Duration) *asyncRetryManager {
	if clock == nil {
		clock = systemClock{}
	}
	if shouldRetry == nil {
		shouldRetry = func(error) bool { return false }
	}
	if backoff == nil {
		backoff = func(attempt int) time.Duration {
			d := 100 * time.Millisecond << min(attempt-1, 5)
			return min(d, 3*time.Second)
		}
	}
	return &asyncRetryManager{
		clock: clock, queues: map[string]*asyncRetryQueue{}, maxAttempts: 8, maxQueue: 1024,
		shouldRetry: shouldRetry, backoff: backoff,
	}
}

func (m *asyncRetryManager) submit(target, messageID string, deliver func(attempt int) error, terminal func(error)) error {
	if target == "" || messageID == "" || deliver == nil {
		return NewError(ErrCodeAtomosInvalidArguments, "AsyncRetry: target, message ID and deliver are required").AddStack(nil)
	}
	m.mutex.Lock()
	if m.closed {
		m.mutex.Unlock()
		return NewError(ErrCodeAtomosIsNotRunning, "AsyncRetry: manager is stopped").AddStack(nil)
	}
	queue := m.queues[target]
	if queue == nil {
		queue = &asyncRetryQueue{}
		m.queues[target] = queue
	}
	if len(queue.requests) >= m.maxQueue {
		m.mutex.Unlock()
		return NewError(ErrCodeCosmosRemoteResponseFailed, "AsyncRetry: target queue is full").AddStack(nil)
	}
	for _, pending := range queue.requests {
		if pending.messageID == messageID {
			m.mutex.Unlock()
			return NewError(ErrCodeOperationConflict, "AsyncRetry: duplicate pending message ID").AddStack(nil)
		}
	}
	queue.requests = append(queue.requests, &asyncRetryRequest{
		target: target, messageID: messageID, deliver: deliver, terminal: terminal, attempt: 1,
	})
	m.scheduleLocked(target, queue, 0)
	m.mutex.Unlock()
	return nil
}

func (m *asyncRetryManager) scheduleLocked(target string, queue *asyncRetryQueue, delay time.Duration) {
	if m.closed || queue.running || queue.timer != nil || len(queue.requests) == 0 {
		return
	}
	queue.running = true
	queue.timer = m.clock.AfterFunc(delay, func() {
		m.run(target, queue)
	})
}

func (m *asyncRetryManager) run(target string, queue *asyncRetryQueue) {
	m.mutex.Lock()
	queue.timer = nil
	if m.closed || len(queue.requests) == 0 {
		queue.running = false
		m.mutex.Unlock()
		return
	}
	queue.executing = true
	request := queue.requests[0]
	m.mutex.Unlock()

	err := request.deliver(request.attempt)

	m.mutex.Lock()
	queue.executing = false
	queue.running = false
	current := m.queues[target]
	terminal := false
	var terminalErr error
	if current != queue || len(queue.requests) == 0 || queue.requests[0] != request {
		m.mutex.Unlock()
		return
	}
	if err == nil || request.attempt >= m.maxAttempts || !m.shouldRetry(err) || m.closed {
		queue.requests = queue.requests[1:]
		terminal = true
		terminalErr = err
	} else {
		request.attempt++
		m.scheduleLocked(target, queue, m.backoff(request.attempt-1))
	}
	if len(queue.requests) == 0 {
		delete(m.queues, target)
	} else {
		m.scheduleLocked(target, queue, 0)
	}
	m.mutex.Unlock()

	if terminal && request.terminal != nil {
		request.terminal(terminalErr)
	}
}

func (m *asyncRetryManager) close() {
	if m == nil {
		return
	}
	m.mutex.Lock()
	m.closed = true
	var terminal []*asyncRetryRequest
	for target, queue := range m.queues {
		if queue.timer != nil {
			queue.timer.Stop()
			queue.timer = nil
			queue.running = false
		}
		start := 0
		if queue.executing {
			start = 1
		}
		terminal = append(terminal, queue.requests[start:]...)
		queue.requests = queue.requests[:start]
		if !queue.executing {
			delete(m.queues, target)
		}
	}
	m.mutex.Unlock()

	for _, request := range terminal {
		if request.terminal != nil {
			request.terminal(NewError(ErrCodeAtomosIsNotRunning, "AsyncRetry: manager stopped before delivery").AddStack(nil))
		}
	}
}
