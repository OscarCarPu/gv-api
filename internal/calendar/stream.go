package calendar

import (
	"sync"
	"time"
)

/*
StreamMessage is what a connected client is told when something moves. It carries the *fact*
of a change and not the change itself: the client refetches the range it is showing, which
keeps one code path for "loaded the page" and "something moved" and cannot drift out of sync
with the database the way an incremental patch stream can.
*/
type StreamMessage struct {
	Type         string    `json:"type"`
	CalendarID   int32     `json:"calendar_id,omitempty"`
	AccountEmail string    `json:"account_email,omitempty"`
	At           time.Time `json:"at"`
}

// Stream is a fan-out hub for the SSE endpoint.
type Stream struct {
	mu     sync.Mutex
	subs   map[int]chan StreamMessage
	nextID int
}

func NewStream() *Stream {
	return &Stream{subs: map[int]chan StreamMessage{}}
}

// Subscribe returns a channel of messages and the function that releases it. The buffer is
// small on purpose: a client that cannot keep up misses messages rather than holding up a
// sync, and the next message it does receive tells it to refetch anyway.
func (s *Stream) Subscribe() (<-chan StreamMessage, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	ch := make(chan StreamMessage, 16)
	s.subs[id] = ch
	return ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if existing, ok := s.subs[id]; ok {
			delete(s.subs, id)
			close(existing)
		}
	}
}

// Publish delivers to every subscriber, never blocking on a slow one.
func (s *Stream) Publish(msg StreamMessage) {
	if msg.At.IsZero() {
		msg.At = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

// Subscribers is the count of live listeners, reported by the sync status endpoint.
func (s *Stream) Subscribers() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subs)
}
