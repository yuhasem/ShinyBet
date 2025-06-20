package state

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var timeBetweenEncounters = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "core_state_encounter_time",
	Help:    "Time (milliseconds) between encounters as measured by receiving post requests",
	Buckets: prometheus.ExponentialBuckets(9000.0, 1.4142, 12),
})
var maxTimeBetweenEncounters = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "core_state_encounter_time_max",
	Help: "Max time (milliseconds) between encounters as measured by receiving post requests",
})

// Listener creates an HTTP server and listens for POST messages to update the
// current state, and notifies registered events of state changes.
type Listener struct {
	server          http.Server
	observers       []Observer
	acl             []string
	lastReceiveTime time.Time
	maxReceiveTime  float64
}

// Observer is the interface Listener expects from events that register for
// updates.
type Observer interface {
	// Notify notifies the Observer of a state change.  Any errors/panics that
	// happen during processing are ignored.  Locking to prevent duplicate or
	// concurrent notifications from stomping is the responsibility of the
	// Observer.
	Notify(s *State)
}

func NewListener(address string, acl []string) (*Listener, error) {
	server := http.Server{}
	l, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	listener := &Listener{
		server:          server,
		observers:       make([]Observer, 0),
		acl:             acl,
		lastReceiveTime: time.Now(),
	}
	http.Handle("/", listener)

	go server.Serve(l)
	slog.Info(fmt.Sprintf("listening on %s", l.Addr()))
	return listener, nil
}

func (l *Listener) ServeHTTP(out http.ResponseWriter, in *http.Request) {
	slog.Debug("state post received", "method", in.Method, "addr", in.RemoteAddr)
	if in.Method != http.MethodPost {
		return
	}
	if !l.checkAcl(in.RemoteAddr) {
		out.WriteHeader(http.StatusUnauthorized)
		return
	}
	rt := time.Now()
	between := float64(rt.UnixMilli() - l.lastReceiveTime.UnixMilli())
	timeBetweenEncounters.Observe(between)
	if between > l.maxReceiveTime {
		maxTimeBetweenEncounters.Set(between)
		l.maxReceiveTime = between
	}
	l.lastReceiveTime = rt

	slog.Debug(fmt.Sprintf("input json: %+v", in.Body))
	state := &State{}
	if err := json.NewDecoder(in.Body).Decode(state); err != nil {
		slog.Info(fmt.Sprintf("decode error: %v", err))
	}
	slog.Debug(fmt.Sprintf("parsed state: %+v", state))

	go func() {
		for _, o := range l.observers {
			// Intentionally serial to prevent database lock contention.
			o.Notify(state)
		}
	}()

	out.WriteHeader(http.StatusOK)
}

// TODO: checkAcl could be improved to have subnet matching or actual IP address
// parsing, but that's not needed for right now.
func (l *Listener) checkAcl(remoteAddr string) bool {
	// If no ACL, accept everything.
	if len(l.acl) == 0 {
		return true
	}
	for _, a := range l.acl {
		if strings.HasPrefix(remoteAddr, a) {
			return true
		}
	}
	return false
}

func (l *Listener) Close() {
	l.server.Shutdown(context.Background())
}

func (l *Listener) Register(o Observer) {
	l.observers = append(l.observers, o)
}
