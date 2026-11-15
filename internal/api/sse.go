package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

func jsonDecode(r io.Reader, v any) error { return json.NewDecoder(r).Decode(v) }

// flushEvery is how often buffered log lines are pushed to the browser. A busy log is hundreds of lines a
// second; flushing each one is a system call and a network write per line.
const flushEvery = 50 * time.Millisecond

// sseWriter writes server-sent events and flushes them in batches, at most every flushEvery.
type sseWriter struct {
	mu    sync.Mutex
	w     io.Writer
	fl    http.Flusher
	dirty bool
	stop  chan struct{}
	done  chan struct{}
}

func newSSE(w http.ResponseWriter) *sseWriter {
	fl, _ := w.(http.Flusher)
	s := &sseWriter{w: w, fl: fl, stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		t := time.NewTicker(flushEvery)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				s.flush()
			case <-s.stop:
				return
			}
		}
	}()
	return s
}

func (s *sseWriter) flush() {
	s.mu.Lock()
	if s.dirty && s.fl != nil {
		s.fl.Flush()
	}
	s.dirty = false
	s.mu.Unlock()
}

// Data queues one message; it goes out with the next batch.
func (s *sseWriter) Data(line string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := fmt.Fprintf(s.w, "data: %s\n\n", line)
	s.dirty = true
	return err
}

// Event writes a named event and flushes right away (the end of the stream, an error).
func (s *sseWriter) Event(name, data string) {
	s.mu.Lock()
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, data)
	s.dirty = true
	s.mu.Unlock()
	s.flush()
}

// Close stops the batching and sends what is left. Call it before the handler returns.
func (s *sseWriter) Close() {
	close(s.stop)
	<-s.done
	s.flush()
}
