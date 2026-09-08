package businesslogic

import "testing"

// The store seeds four public todos and one private one, so a signed-out
// visitor may see four and a signed-in visitor five.

func TestTodosAndCountAgree(t *testing.T) {
	s := NewStore()

	for _, signedIn := range []bool{false, true} {
		listed := len(s.Todos(signedIn))
		_, unsubscribe, counted := s.Watch(signedIn)
		unsubscribe()

		if counted.Count != listed {
			t.Errorf("signedIn=%v: Watch reported %d, Todos listed %d", signedIn, counted.Count, listed)
		}
	}

	if got := len(s.Todos(false)); got != 4 {
		t.Errorf("a signed-out visitor sees %d todos, want 4", got)
	}
	if got := len(s.Todos(true)); got != 5 {
		t.Errorf("a signed-in visitor sees %d todos, want 5", got)
	}
}

func TestEachWatcherIsToldItsOwnCount(t *testing.T) {
	s := NewStore()

	out, stopOut, startOut := s.Watch(false)
	defer stopOut()
	in, stopIn, startIn := s.Watch(true)
	defer stopIn()

	if startOut.Count != 4 || startIn.Count != 5 {
		t.Fatalf("opening counts = %d signed out, %d signed in; want 4 and 5", startOut.Count, startIn.Count)
	}

	// One added todo is public, so both watchers gain exactly one — and
	// neither is told about the other's rows.
	s.Add("a public todo")

	if got := <-out; got.Count != 5 {
		t.Errorf("the signed-out watcher was sent %d, want 5", got.Count)
	}
	if got := <-in; got.Count != 6 {
		t.Errorf("the signed-in watcher was sent %d, want 6", got.Count)
	}
	if got := len(s.Todos(false)); got != 5 {
		t.Errorf("a signed-out visitor now lists %d todos, want 5", got)
	}
}

func TestUnsubscribingStopsTheUpdates(t *testing.T) {
	s := NewStore()

	ch, stop, _ := s.Watch(false)
	stop()
	s.Add("nobody is listening")

	select {
	case v := <-ch:
		t.Errorf("an unsubscribed watcher was sent %+v", v)
	default:
	}
}

// The write path is guarded by the same rule as the read path. Signed out,
// getTodo already refuses the private todo; Rename used to change it anyway
// and hand the whole record — text, id and `private: true` — back to the
// visitor who was not allowed to know it existed.
func TestRenameObeysTheSameRuleAsTheReaders(t *testing.T) {
	s := NewStore()

	private, ok := s.Todo("t3", true)
	if !ok || !private.Private {
		t.Fatalf("t3 is meant to be the private fixture; got %+v ok=%v", private, ok)
	}

	if _, ok := s.Rename("t3", "renamed by a signed-out visitor", false); ok {
		t.Error("Rename accepted a signed-out visitor's change to a private todo")
	}
	if got, _ := s.Todo("t3", true); got.Text != private.Text {
		t.Errorf("the private todo now reads %q; a signed-out visitor changed it", got.Text)
	}

	// The same visitor may still rename what they can see, and a signed-in
	// visitor may rename the private one.
	if _, ok := s.Rename("t1", "renamed by anyone", false); !ok {
		t.Error("Rename refused a public todo to a signed-out visitor")
	}
	if _, ok := s.Rename("t3", "renamed by ada", true); !ok {
		t.Error("Rename refused the private todo to a signed-in visitor")
	}
}

// The live board shows the newest todo a visitor may see beside the count, so
// the two are one value: a board that paired a count from one moment with a
// newest from another would be a page that never existed.
func TestTheSnapshotNamesTheNewestTodoTheVisitorMaySee(t *testing.T) {
	s := NewStore()

	out, stopOut, startOut := s.Watch(false)
	defer stopOut()
	in, stopIn, startIn := s.Watch(true)
	defer stopIn()

	// The private todo is the newest of the five seeded rows only for a
	// signed-in visitor; a signed-out one is never told it is there.
	if startOut.Newest != "the right of the pair" {
		t.Errorf("a signed-out visitor's newest is %q", startOut.Newest)
	}
	if startIn.Newest != "the right of the pair" {
		t.Errorf("a signed-in visitor's newest is %q", startIn.Newest)
	}

	s.Add("the latest arrival")
	if got := <-out; got.Newest != "the latest arrival" || got.Count != 5 {
		t.Errorf("the signed-out watcher was sent %+v", got)
	}
	if got := <-in; got.Newest != "the latest arrival" || got.Count != 6 {
		t.Errorf("the signed-in watcher was sent %+v", got)
	}

	// A rename changes what the newest todo says, and wakes the watchers too.
	s.Rename("t6", "renamed after arriving", false)
	if got := <-out; got.Newest != "renamed after arriving" {
		t.Errorf("a rename left the board saying %q", got.Newest)
	}
}
