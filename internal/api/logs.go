package api

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
)

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	k, okc := s.cluster(w, r)
	if !okc {
		return
	}
	q := r.URL.Query()
	tail, _ := strconv.ParseInt(q.Get("tail"), 10, 64)
	since, _ := strconv.ParseInt(q.Get("since"), 10, 64)
	rc, err := k.Logs(r.Context(), r.PathValue("ns"), r.PathValue("pod"), q.Get("container"), tail, since, q.Get("follow") == "true", q.Get("timestamps") == "true", q.Get("previous") == "true")
	if err != nil {
		kubeFail(w, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	sse := newSSE(w)
	defer sse.Close()
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		if sse.Data(sc.Text()) != nil {
			return
		}
	}
	// Tell the UI the stream ended on purpose, so it does not reconnect.
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) && r.Context().Err() == nil {
		sse.Event("error", err.Error())
	} else {
		sse.Event("end", "")
	}
}

// maxLogPods caps how many pods one merged log stream opens.
const maxLogPods = 30

// workloadLogs streams the logs of every pod a deployment or service selects as one SSE stream.
// Each line is tagged "[pod]" ("[pod/container]" when a pod has several and none was chosen);
// lines of different pods arrive in the order they are read, not sorted by time.
func (s *Server) workloadLogs(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		k, okc := s.cluster(w, r)
		if !okc {
			return
		}
		q := r.URL.Query()
		tail, _ := strconv.ParseInt(q.Get("tail"), 10, 64)
		since, _ := strconv.ParseInt(q.Get("since"), 10, 64)
		follow, ts, prev := q.Get("follow") == "true", q.Get("timestamps") == "true", q.Get("previous") == "true"
		ns, name, want := r.PathValue("ns"), r.PathValue("name"), q.Get("container")
		pods, err := k.PodsOf(r.Context(), kind, ns, name)
		if err != nil {
			kubeFail(w, err)
			return
		}
		type src struct{ pod, container, label string }
		var srcs []src
		for _, p := range pods {
			if p.Status.Phase == corev1.PodPending {
				continue
			}
			for _, c := range p.Spec.Containers {
				if want != "" && want != c.Name {
					continue
				}
				label := p.Name
				if want == "" && len(p.Spec.Containers) > 1 {
					label += "/" + c.Name
				}
				srcs = append(srcs, src{p.Name, c.Name, label})
			}
		}
		if len(srcs) == 0 {
			fail(w, http.StatusNotFound, errors.New("no running pods to read logs from"))
			return
		}
		note := ""
		if len(srcs) > maxLogPods {
			note = fmt.Sprintf("[showing %d of %d containers]", maxLogPods, len(srcs))
			srcs = srcs[:maxLogPods]
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		sse := newSSE(w)
		defer sse.Close()
		ctx := r.Context()
		lines := make(chan string, 256)
		send := func(l string) {
			select {
			case lines <- l:
			case <-ctx.Done():
			}
		}
		tag := func(label, line string) string {
			if ts { // keep the timestamp first: the UI reads it from the start of the line
				if i := strings.IndexByte(line, ' '); i > 0 {
					return line[:i] + " [" + label + "] " + line[i+1:]
				}
			}
			return "[" + label + "] " + line
		}
		var wg sync.WaitGroup
		for _, c := range srcs {
			wg.Add(1)
			go func(c src) {
				defer wg.Done()
				rc, err := k.Logs(ctx, ns, c.pod, c.container, tail, since, follow, ts, prev)
				if err != nil {
					send("[" + c.label + "] [error: " + err.Error() + "]")
					return
				}
				defer rc.Close()
				sc := bufio.NewScanner(rc)
				sc.Buffer(make([]byte, 64*1024), 1024*1024)
				for sc.Scan() {
					send(tag(c.label, sc.Text()))
				}
			}(c)
		}
		go func() { wg.Wait(); close(lines) }()
		if note != "" {
			sse.Data(note)
		}
		for l := range lines {
			if sse.Data(l) != nil {
				return
			}
		}
		sse.Event("end", "")
	}
}
