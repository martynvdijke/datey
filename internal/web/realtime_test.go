package web

import (
	"testing"
	"time"
)

func TestBrokerFanOut(t *testing.T) {
	b := NewBroker()

	ch1, cancel1 := b.Subscribe(1)
	defer cancel1()
	ch2, cancel2 := b.Subscribe(2)
	defer cancel2()

	n := Notification{Title: "New person", Message: "Dana added", URL: "/people/7", Kind: "person", At: time.Now()}
	b.Publish([]int{1}, n)

	select {
	case got := <-ch1:
		if got.Title != n.Title || got.Kind != n.Kind || got.URL != n.URL || got.Message != n.Message {
			t.Errorf("user 1 got %+v, want %+v", got, n)
		}
	case <-time.After(time.Second):
		t.Fatal("user 1 did not receive the notification")
	}

	select {
	case got := <-ch2:
		t.Errorf("user 2 must not receive user 1's notification, got %+v", got)
	case <-time.After(50 * time.Millisecond):
		// expected: nothing for user 2.
	}
}

func TestBrokerFanOutMultipleSubscribersSameUser(t *testing.T) {
	b := NewBroker()
	ch1, cancel1 := b.Subscribe(5)
	defer cancel1()
	ch2, cancel2 := b.Subscribe(5)
	defer cancel2()

	b.PublishNewItem([]int{5}, "person", "New person", "Dana added", "/people/7")

	for i, ch := range []<-chan Notification{ch1, ch2} {
		select {
		case got := <-ch:
			if got.Kind != "person" || got.URL != "/people/7" {
				t.Errorf("subscriber %d got %+v", i, got)
			}
			if got.At.IsZero() {
				t.Errorf("subscriber %d: At must be set", i)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d did not receive the notification", i)
		}
	}
}

func TestBrokerDropOnFull(t *testing.T) {
	b := NewBroker()
	ch, cancel := b.Subscribe(1)
	defer cancel()

	// No reader: the first brokerBuffer messages fit, the rest must be dropped
	// without blocking the publisher.
	done := make(chan struct{})
	go func() {
		for i := 0; i < brokerBuffer*5; i++ {
			b.Publish([]int{1}, Notification{Title: "t"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a full subscriber")
	}

	if got := len(ch); got != brokerBuffer {
		t.Errorf("buffered messages = %d, want %d", got, brokerBuffer)
	}
}

func TestBrokerUnsubscribe(t *testing.T) {
	b := NewBroker()
	ch, cancel := b.Subscribe(1)

	cancel()
	cancel() // must be safe to call twice

	if _, ok := <-ch; ok {
		t.Error("channel should be closed after cancel")
	}

	b.mu.RLock()
	subs := len(b.subs)
	b.mu.RUnlock()
	if subs != 0 {
		t.Errorf("subscriptions after cancel = %d, want 0", subs)
	}

	// Publishing to an unsubscribed user must not panic or block.
	b.Publish([]int{1}, Notification{Title: "t"})
}

func TestBrokerNilAndEmptyPublish(t *testing.T) {
	var b *Broker
	b.Publish([]int{1}, Notification{Title: "t"}) // nil broker: no-op

	live := NewBroker()
	live.Publish(nil, Notification{Title: "t"})
	live.Publish([]int{}, Notification{Title: "t"})
}
