package engine

import (
	"sync"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// bus fans events out to subscribers without ever blocking the publisher.
type bus struct {
	mu     sync.Mutex
	subs   map[chan event.Event]struct{}
	closed bool
}

func newBus() *bus {
	return &bus{subs: map[chan event.Event]struct{}{}}
}

func (b *bus) subscribe(buf int) (<-chan event.Event, func()) {
	ch := make(chan event.Event, buf)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		close(ch)
		return ch, func() {}
	}
	b.subs[ch] = struct{}{}
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if _, ok := b.subs[ch]; ok {
				delete(b.subs, ch)
				close(ch)
			}
		})
	}
}

// publish delivers ev to every subscriber with room in its buffer and drops
// it for the rest.
func (b *bus) publish(ev event.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (b *bus) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for ch := range b.subs {
		delete(b.subs, ch)
		close(ch)
	}
}
