package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// A browser that stopped reading must not block the shell: Write gives up after the write timeout,
// drops that connection and returns, so the lock is free for the next attach.
func TestShellWriteDoesNotHangOnAStuckBrowser(t *testing.T) {
	old := writeTimeout
	writeTimeout = 200 * time.Millisecond
	defer func() { writeTimeout = old }()

	serverConn := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			serverConn <- c
		}
	}))
	defer srv.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close() // never read from: its TCP buffers fill up

	sh := &shell{sub: <-serverConn}
	chunk := make([]byte, 256<<10)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 400 && sh.sub != nil; i++ { // up to ~100 MB: far more than the socket buffers hold
			sh.Write(chunk)
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Write is stuck on a browser that does not read")
	}
	sh.mu.Lock()
	detached := sh.sub == nil
	sh.mu.Unlock()
	if !detached {
		t.Fatal("the stuck connection was not dropped")
	}
}
