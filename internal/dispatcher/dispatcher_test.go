package dispatcher

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestDispatcherSerializesSameKey(t *testing.T) {
	t.Parallel()

	d := New(8, time.Second)
	defer d.Shutdown(context.Background())

	ctx := context.Background()
	var mu sync.Mutex
	order := make([]int, 0, 3)
	done := make(chan struct{})

	for i := 0; i < 3; i++ {
		index := i
		if err := d.Submit(ctx, "user:1", func(context.Context) {
			mu.Lock()
			order = append(order, index)
			if len(order) == 3 {
				close(done)
			}
			mu.Unlock()
		}); err != nil {
			t.Fatalf("submit: %v", err)
		}
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ordered execution")
	}

	mu.Lock()
	defer mu.Unlock()
	for i, value := range order {
		if value != i {
			t.Fatalf("unexpected order %v", order)
		}
	}
}

func TestDispatcherRunsDifferentKeysInParallel(t *testing.T) {
	t.Parallel()

	d := New(8, time.Second)
	defer d.Shutdown(context.Background())

	ctx := context.Background()
	start := make(chan struct{})
	finished := make(chan struct{}, 2)

	for _, key := range []string{"user:1", "user:2"} {
		key := key
		if err := d.Submit(ctx, key, func(context.Context) {
			<-start
			time.Sleep(100 * time.Millisecond)
			finished <- struct{}{}
		}); err != nil {
			t.Fatalf("submit: %v", err)
		}
	}

	close(start)
	deadline := time.After(300 * time.Millisecond)
	count := 0
	for count < 2 {
		select {
		case <-finished:
			count++
		case <-deadline:
			t.Fatal("expected parallel execution across mailbox keys")
		}
	}
}

func TestDispatcherEvictsIdleMailbox(t *testing.T) {
	t.Parallel()

	d := New(1, 50*time.Millisecond)
	defer d.Shutdown(context.Background())

	if err := d.Submit(context.Background(), "user:1", func(context.Context) {}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	d.mu.Lock()
	_, ok := d.mailboxes["user:1"]
	d.mu.Unlock()
	if ok {
		t.Fatal("expected idle mailbox eviction")
	}
}
