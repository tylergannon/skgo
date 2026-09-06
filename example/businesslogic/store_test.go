package businesslogic

import "testing"

// The store seeds two public todos and one private one, so a signed-out
// visitor may see two and a signed-in visitor three.

func TestTodosAndCountAgree(t *testing.T) {
	s := NewStore()

	for _, signedIn := range []bool{false, true} {
		listed := len(s.Todos(signedIn))
		_, unsubscribe, counted := s.Watch(signedIn)
		unsubscribe()

		if counted != listed {
			t.Errorf("signedIn=%v: Watch reported %d, Todos listed %d", signedIn, counted, listed)
		}
	}

	if got := len(s.Todos(false)); got != 2 {
		t.Errorf("a signed-out visitor sees %d todos, want 2", got)
	}
	if got := len(s.Todos(true)); got != 3 {
		t.Errorf("a signed-in visitor sees %d todos, want 3", got)
	}
}

func TestEachWatcherIsToldItsOwnCount(t *testing.T) {
	s := NewStore()

	out, stopOut, startOut := s.Watch(false)
	defer stopOut()
	in, stopIn, startIn := s.Watch(true)
	defer stopIn()

	if startOut != 2 || startIn != 3 {
		t.Fatalf("opening counts = %d signed out, %d signed in; want 2 and 3", startOut, startIn)
	}

	// One added todo is public, so both watchers gain exactly one — and
	// neither is told about the other's rows.
	s.Add("a public todo")

	if got := <-out; got != 3 {
		t.Errorf("the signed-out watcher was sent %d, want 3", got)
	}
	if got := <-in; got != 4 {
		t.Errorf("the signed-in watcher was sent %d, want 4", got)
	}
	if got := len(s.Todos(false)); got != 3 {
		t.Errorf("a signed-out visitor now lists %d todos, want 3", got)
	}
}

func TestUnsubscribingStopsTheUpdates(t *testing.T) {
	s := NewStore()

	ch, stop, _ := s.Watch(false)
	stop()
	s.Add("nobody is listening")

	select {
	case v := <-ch:
		t.Errorf("an unsubscribed watcher was sent %d", v)
	default:
	}
}
