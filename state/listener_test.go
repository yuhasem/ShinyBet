package state

import (
	"net/http"
	"strings"
	"testing"
)

type NoResponseWriter struct{}

func (NoResponseWriter) Header() http.Header       { return nil }
func (NoResponseWriter) Write([]byte) (int, error) { return 0, nil }
func (NoResponseWriter) WriteHeader(int)           {}

type StringReadCloser struct {
	s *strings.Reader
}

func (s *StringReadCloser) Read(p []byte) (n int, err error) { return s.s.Read(p) }
func (s *StringReadCloser) Close() error                     { return nil }

// TestObserver copies state sent over Notify into itself, since Listener
// doesn't store a state object.
type TestObserver struct {
	recv chan struct{}
	s    *State
}

func (t *TestObserver) Notify(s *State) {
	t.s = s
	t.recv <- struct{}{}
}

func TestDecode(t *testing.T) {
	l, _ := NewListener("localhost:8008", []string{})
	defer l.Close()
	o := TestObserver{recv: make(chan struct{})}
	l.Register(&o)
	in := http.Request{
		Body: &StringReadCloser{s: strings.NewReader(`
		{
			"encounter": {
				"is_shiny": false,
				"is_anti_shiny": false,
				"species": {"name": "pokeyman"},
				"held_item": {"name": "the stuff"}
			}
		}
		`)},
		Method: "POST",
	}
	l.ServeHTTP(NoResponseWriter{}, &in)
	<-o.recv
	if o.s.Encounter.HeldItem.Name != "the stuff" {
		t.Errorf("Decoded held item %s, want 'the stuff'", o.s.Encounter.HeldItem.Name)
	}

	in = http.Request{
		Body: &StringReadCloser{s: strings.NewReader(`
		{
			"encounter": {
				"is_shiny": false,
				"is_anti_shiny": false,
				"species": {"name": "pokeyman"},
				"held_item": None
			}
		}
		`)},
		Method: "POST",
	}
	l.ServeHTTP(NoResponseWriter{}, &in)
	<-o.recv
	if o.s.Encounter.HeldItem.Name != "" {
		t.Errorf("Decoded held item %s, want None", o.s.Encounter.HeldItem.Name)
	}
}
