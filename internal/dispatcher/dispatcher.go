package dispatcher

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrDispatcherClosed = errors.New("dispatcher closed")

type job struct {
	ctx context.Context
	fn  func(context.Context)
}

type mailbox struct {
	jobs   chan job
	mu     sync.Mutex
	closed bool
}

type Dispatcher struct {
	buffer  int
	idleTTL time.Duration

	mu        sync.Mutex
	mailboxes map[string]*mailbox
	closed    bool
	wg        sync.WaitGroup
}

func New(buffer int, idleTTL time.Duration) *Dispatcher {
	if buffer <= 0 {
		buffer = 1
	}
	if idleTTL <= 0 {
		idleTTL = 5 * time.Minute
	}
	return &Dispatcher{
		buffer:    buffer,
		idleTTL:   idleTTL,
		mailboxes: make(map[string]*mailbox),
	}
}

func (d *Dispatcher) Submit(ctx context.Context, key string, fn func(context.Context)) error {
	if fn == nil {
		return nil
	}
	for {
		mb, err := d.getOrCreateMailbox(key)
		if err != nil {
			return err
		}
		mb.mu.Lock()
		if mb.closed {
			mb.mu.Unlock()
			continue
		}
		select {
		case <-ctx.Done():
			mb.mu.Unlock()
			return ctx.Err()
		case mb.jobs <- job{ctx: ctx, fn: fn}:
			mb.mu.Unlock()
			return nil
		}
	}
}

func (d *Dispatcher) Shutdown(ctx context.Context) error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	mailboxes := make([]*mailbox, 0, len(d.mailboxes))
	for _, mb := range d.mailboxes {
		mailboxes = append(mailboxes, mb)
	}
	d.mailboxes = make(map[string]*mailbox)
	d.mu.Unlock()

	for _, mb := range mailboxes {
		mb.mu.Lock()
		if !mb.closed {
			mb.closed = true
			close(mb.jobs)
		}
		mb.mu.Unlock()
	}

	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (d *Dispatcher) getOrCreateMailbox(key string) (*mailbox, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, ErrDispatcherClosed
	}
	if mb, ok := d.mailboxes[key]; ok {
		return mb, nil
	}
	mb := &mailbox{jobs: make(chan job, d.buffer)}
	d.mailboxes[key] = mb
	d.wg.Add(1)
	go d.runMailbox(key, mb)
	return mb, nil
}

func (d *Dispatcher) runMailbox(key string, mb *mailbox) {
	defer d.wg.Done()

	timer := time.NewTimer(d.idleTTL)
	defer timer.Stop()

	for {
		select {
		case item, ok := <-mb.jobs:
			if !ok {
				return
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			item.fn(item.ctx)
			timer.Reset(d.idleTTL)
		case <-timer.C:
			if d.tryEvictMailbox(key, mb) {
				return
			}
			timer.Reset(d.idleTTL)
		}
	}
}

func (d *Dispatcher) tryEvictMailbox(key string, mb *mailbox) bool {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if mb.closed || len(mb.jobs) > 0 {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		mb.closed = true
		close(mb.jobs)
		return true
	}
	current, ok := d.mailboxes[key]
	if !ok || current != mb {
		return false
	}
	delete(d.mailboxes, key)
	mb.closed = true
	close(mb.jobs)
	return true
}
