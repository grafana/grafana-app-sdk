package operator

import (
	"context"
	"slices"
	"sync"
	"time"
)

// The legacy ticker runs callbacks serially. Each key keeps its due requests
// visible to dequeue until they start, and its running request until settled.
type tickerRetryQueue struct {
	mu      sync.Mutex
	pending []queuedRetry
	ready   []queuedRetry
	running *tickerRetryAttempt
	retired bool
}

type tickerRetryAttempt struct {
	done    chan struct{}
	discard bool
	events  []retryDequeueEvent
}

func (c *InformerController) enqueueRetry(req RetryRequest, syncRetry *watcherSyncRetry) {
	if c.retryProcessor != nil {
		c.retryProcessor.enqueue(req, syncRetry)
		return
	}
	// A callback can mutate its object while a newer event inspects the retry.
	if req.Object != nil {
		req.Object = req.Object.Copy()
	}
	for {
		q, _ := c.toRetry.LoadOrCompute(req.Key, func() (*tickerRetryQueue, bool) { return &tickerRetryQueue{}, false })
		q.mu.Lock()
		if !q.retired {
			q.pending = append(q.pending, queuedRetry{req, syncRetry})
			q.mu.Unlock()
			return
		}
		// The ticker removed this empty queue between lookup and lock acquisition.
		q.mu.Unlock()
	}
}

func (q *tickerRetryQueue) dequeue(event retryDequeueEvent) []<-chan struct{} {
	q.mu.Lock()
	defer q.mu.Unlock()
	matches := func(req queuedRetry) bool {
		discard, _ := event.decide(req)
		return discard
	}
	q.pending = slices.DeleteFunc(q.pending, matches)
	q.ready = slices.DeleteFunc(q.ready, matches)
	attempt := q.running
	if attempt == nil {
		return nil
	}
	if event.policy == nil {
		attempt.discard = true
		attempt.events = nil
	} else if !attempt.discard {
		// The policy used to see a rescheduled retry after the callback
		// returned. Preserve both that error and the incoming event snapshot.
		event.object = event.object.Copy()
		attempt.events = append(attempt.events, event)
	}
	return []<-chan struct{}{attempt.done}
}

func (c *InformerController) retryTicker(ctx context.Context) {
	ticker := time.NewTicker(c.retryTickerInterval)
	defer ticker.Stop()
	for {
		select {
		case t := <-ticker.C:
			for key, q := range c.toRetry.All() {
				c.runTickerRetries(ctx, key, q, t)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (c *InformerController) runTickerRetries(ctx context.Context, key string, q *tickerRetryQueue, tick time.Time) {
	q.mu.Lock()
	if q.retired || q.running != nil {
		q.mu.Unlock()
		return
	}
	// Take only work due at this tick. Enqueues and reschedules during
	// callbacks become eligible on a subsequent tick.
	q.pending = slices.DeleteFunc(q.pending, func(req queuedRetry) bool {
		if !tick.After(req.RetryAfter) {
			return false
		}
		q.ready = append(q.ready, req)
		return true
	})
	for len(q.ready) > 0 && ctx.Err() == nil {
		req := q.ready[0]
		q.ready[0] = queuedRetry{}
		q.ready = q.ready[1:]
		attempt := &tickerRetryAttempt{done: make(chan struct{})}
		q.running = attempt
		q.mu.Unlock()

		requeue := c.runTickerAttempt(&req, tick)

		q.mu.Lock()
		if requeue && attempt.keep(req) {
			q.pending = append(q.pending, req)
		}
		// A waiter may dispatch cleanup as soon as done closes.
		q.running = nil
		close(attempt.done)
	}
	if len(q.pending) == 0 && len(q.ready) == 0 {
		q.retired = true
		c.toRetry.Delete(key)
	}
	q.mu.Unlock()
}

func (a *tickerRetryAttempt) keep(req queuedRetry) bool {
	if a.discard {
		return false
	}
	for _, event := range a.events {
		if discard, _ := event.decide(req); discard {
			return false
		}
	}
	return true
}

func (c *InformerController) runTickerAttempt(req *queuedRetry, tick time.Time) bool {
	after, err := req.RetryFunc()
	req.LastError = err
	if after != nil {
		req.RetryAfter = tick.Add(*after)
		return true
	}
	if err == nil || c.RetryPolicy == nil {
		return false
	}
	req.Attempt++
	ok, delay := c.RetryPolicy(err, req.Attempt)
	req.RetryAfter = tick.Add(delay)
	return ok
}
