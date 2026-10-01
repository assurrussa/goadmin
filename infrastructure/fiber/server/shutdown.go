package server

import "sync"

// Shutdown signals HTTP shutdown to long-lived connections.
type Shutdown struct {
	channel chan struct{}
	once    sync.Once
}

func NewShutdown() *Shutdown {
	return &Shutdown{channel: make(chan struct{})}
}

func (s *Shutdown) Read() <-chan struct{} {
	return s.channel
}

func (s *Shutdown) Close() {
	s.once.Do(func() { close(s.channel) })
}
