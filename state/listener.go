package state

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
)

// Listener creates an HTTP server and listens for POST messages to update the
// current state, and notifies registered events of state changes.
type Listener struct {
	state     *State
	server    http.Server
	observers []Observer
	acl       []string
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

	state := &State{}
	listener := &Listener{
		state:     state,
		server:    server,
		observers: make([]Observer, 0),
		acl:       acl,
	}
	http.Handle("/", listener)

	go server.Serve(l)
	log.Printf("listening on %s", l.Addr())
	return listener, nil
}

func (l *Listener) ServeHTTP(out http.ResponseWriter, in *http.Request) {
	if in.Method != http.MethodPost {
		return
	}
	// Print this so I can ACL later.
	fmt.Println(in.RemoteAddr)
	if !l.checkAcl(in.RemoteAddr) {
		out.WriteHeader(http.StatusUnauthorized)
		return
	}
	// fmt.Println(in.ContentLength)
	if err := json.NewDecoder(in.Body).Decode(l.state); err != nil {
		fmt.Printf("decode error: %v", err)
	}
	// The stats object sent from pokebot does include the current encounter in
	// the phase, which leads to an off by one error from what's the actual
	// reported phase on stream.
	l.state.Stats.CurrentPhase.Encounters++
	// fmt.Printf("%+v\n", l.state)

	go func() {
		for _, o := range l.observers {
			go o.Notify(l.state)
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
