package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/bondhansarker/devbuddy/internal/kube"
	"github.com/gorilla/websocket"
	"k8s.io/client-go/tools/remotecommand"
)

// Origin is already checked by sameOrigin.
var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

const shellBacklog = 256 << 10

// shell is one pod shell that outlives the browser: it keeps running when the page is
// refreshed or closed, keeps the last output, and the next WebSocket re-attaches to it.
type shell struct {
	cluster, ns, pod, container string
	mu                          sync.Mutex // guards the fields below and every write to sub
	buf                         []byte
	sub                         *websocket.Conn
	done                        bool
	in                          *io.PipeWriter
	size                        chan remotecommand.TerminalSize
	stop                        context.CancelFunc
}

// send writes one frame with a deadline: a browser that stopped reading (asleep tab, dead network) must
// not hold sh.mu, because the pod's output and every re-attach wait behind it.
var writeTimeout = 5 * time.Second

func send(c *websocket.Conn, data []byte) error {
	_ = c.SetWriteDeadline(time.Now().Add(writeTimeout))
	return c.WriteMessage(websocket.BinaryMessage, data)
}

// Write is the shell's stdout: keep it for a later attach, and send it to the page if one is attached.
func (sh *shell) Write(p []byte) (int, error) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.buf = append(sh.buf, p...)
	if n := len(sh.buf) - shellBacklog; n > 0 {
		sh.buf = append([]byte(nil), sh.buf[n:]...)
	}
	if sh.sub != nil && send(sh.sub, p) != nil {
		sh.sub = nil
	}
	return len(p), nil
}

// shellFor returns the running shell for the pod container, starting one if there is none.
func (s *Server) shellFor(k *kube.Client, ns, pod, container string) *shell {
	key := k.Name + "/" + ns + "/" + pod + "/" + container
	s.shMu.Lock()
	defer s.shMu.Unlock()
	if sh := s.shells[key]; sh != nil {
		return sh
	}
	pr, pw := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	sh := &shell{cluster: k.Name, ns: ns, pod: pod, container: container, in: pw, size: make(chan remotecommand.TerminalSize, 4), stop: cancel}
	s.shells[key] = sh
	go func() {
		err := k.Shell(ctx, ns, pod, container, kube.Terminal{In: pr, Out: sh, Resize: sh.size})
		msg := "\r\n[session closed]\r\n"
		if err != nil && ctx.Err() == nil {
			msg = "\r\n[error: " + err.Error() + "]\r\n"
		}
		s.shMu.Lock()
		if s.shells[key] == sh {
			delete(s.shells, key)
		}
		s.shMu.Unlock()
		sh.Write([]byte(msg))
		sh.mu.Lock()
		sh.done = true
		if sh.sub != nil {
			sh.sub.Close()
		}
		sh.mu.Unlock()
		pw.Close()
		cancel()
	}()
	return sh
}

// exec attaches a WebSocket to the pod's shell: binary frames are stdin/stdout,
// a text frame {"cols":n,"rows":n} is a resize. Closing the socket only detaches.
func (s *Server) exec(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReadOnly {
		fail(w, http.StatusForbidden, errReadOnly)
		return
	}
	k, okc := s.cluster(w, r) // before the upgrade: a missing connection is a plain JSON error
	if !okc {
		return
	}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	c.SetReadLimit(1 << 20) // keystrokes and pastes, not unbounded frames
	sh := s.shellFor(k, r.PathValue("ns"), r.PathValue("pod"), r.URL.Query().Get("container"))

	sh.mu.Lock()
	if sh.sub != nil {
		sh.sub.Close() // another page took over
	}
	if len(sh.buf) > 0 {
		_ = send(c, sh.buf)
	}
	if sh.done {
		sh.mu.Unlock()
		return
	}
	sh.sub = c
	sh.mu.Unlock()
	defer func() {
		sh.mu.Lock()
		if sh.sub == c {
			sh.sub = nil
		}
		sh.mu.Unlock()
	}()

	for {
		typ, msg, err := c.ReadMessage()
		if err != nil {
			return
		}
		if typ == websocket.BinaryMessage {
			if _, err := sh.in.Write(msg); err != nil {
				return
			}
			continue
		}
		var sz struct{ Cols, Rows uint16 }
		if json.Unmarshal(msg, &sz) == nil && sz.Cols > 0 && sz.Rows > 0 {
			select {
			case sh.size <- remotecommand.TerminalSize{Width: sz.Cols, Height: sz.Rows}:
			default:
			}
		}
	}
}

// shells lists the running shells so a reloaded page can reopen its tabs.
func (s *Server) shellList(w http.ResponseWriter, r *http.Request) {
	s.shMu.Lock()
	out := []map[string]string{}
	for _, sh := range s.shells {
		out = append(out, map[string]string{"cluster": sh.cluster, "namespace": sh.ns, "pod": sh.pod, "container": sh.container})
	}
	s.shMu.Unlock()
	ok(w, out)
}

// shellKill ends a shell for good (the tab was closed).
func (s *Server) shellKill(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	s.shMu.Lock()
	name := q.Get("cluster")
	if name == "" && len(s.clusters) > 0 {
		name = s.clusters[0].Name
	}
	sh := s.shells[name+"/"+q.Get("namespace")+"/"+q.Get("pod")+"/"+q.Get("container")]
	s.shMu.Unlock()
	if sh != nil {
		sh.stop()
	}
	ok(w, map[string]bool{"ok": true})
}
