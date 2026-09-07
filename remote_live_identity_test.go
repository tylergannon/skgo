package skgo

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tylergannon/polytype/devalue"
	"github.com/tylergannon/skgo/internal/remotearg"
)

// These tests drive a live query over the real endpoint and read the frames as
// they cross the wire. A page can tell you the number it displays; only the
// frames can tell you whether the stream sent one value or raced to it, and
// whether a reconnect dropped a value or delivered it twice.

// liveSessionCookie stands in for the example app's session cookie.
const liveSessionCookie = "session"

func liveSignedIn(ctx context.Context) bool {
	user, _ := EventFrom(ctx).Cookie(liveSessionCookie)
	return user != ""
}

// rows is an app whose data is filtered by identity: two rows anybody may see,
// one only a signed-in visitor may see, plus whatever commands have added.
type rows struct {
	mu     sync.Mutex
	added  int
	subs   map[chan struct{}]struct{}
	opened int
	closed int
}

func newRows() *rows { return &rows{subs: map[chan struct{}]struct{}{}} }

// count is the number of rows this visitor may see.
func (r *rows) count(signedIn bool) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 2 + r.added
	if signedIn {
		n++
	}
	return n
}

// add appends a public row and wakes every watcher.
func (r *rows) add() {
	r.mu.Lock()
	r.added++
	for ch := range r.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	r.mu.Unlock()
}

// watch registers a subscription and returns the function that ends it. The
// counters let a test prove a producer was torn down rather than leaked.
func (r *rows) watch() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	r.mu.Lock()
	r.subs[ch] = struct{}{}
	r.opened++
	r.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			r.mu.Lock()
			delete(r.subs, ch)
			r.closed++
			r.mu.Unlock()
		})
	}
}

func (r *rows) watching() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.subs)
}

func (r *rows) lifecycle() (opened, closed int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.opened, r.closed
}

// liveApp is the fixture: a live count, a plain query over the same data, a
// command that mutates it, and the two commands that change who is asking.
type liveApp struct {
	rs         *Remotes
	rows       *rows
	srv        *httptest.Server
	frames     *atomic.Int32
	watchCount *Remote
	getRows    *Remote
	addRow     *Remote
	signIn     *Remote
	signOut    *Remote
}

func newLiveApp(t *testing.T) *liveApp {
	t.Helper()
	data := newRows()

	watchCountFn := func(ctx context.Context, yield func(int) error) error {
		// The identity is read once, here. Kit's event is a snapshot of the
		// request that opened the stream and is never rebuilt for a later
		// yield, so there is no later request to consult.
		signedIn := liveSignedIn(ctx)
		ticks, stop := data.watch()
		defer stop()

		if err := yield(data.count(signedIn)); err != nil {
			return err
		}
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticks:
				if err := yield(data.count(signedIn)); err != nil {
					return err
				}
			}
		}
	}
	watchCount := NewLiveQueryNoArg(testModule, "watchCount", watchCountFn)

	getRowsFn := func(ctx context.Context) (int, error) {
		return data.count(liveSignedIn(ctx)), nil
	}
	getRows := NewQueryNoArg(testModule, "getRows", getRowsFn)

	// The two commands that change who is asking accept what a page asks for
	// when the identity moves: the plain query re-runs, and the live query —
	// which cannot re-read a cookie — reconnects. A command that named neither
	// would refresh neither, however loudly the client asked.
	identityChanged := func(ctx context.Context) error {
		if err := RefreshRequestedNoArg(ctx, getRowsFn); err != nil {
			return err
		}
		return ReconnectRequestedNoArg(ctx, watchCountFn)
	}

	addRow := NewCommandNoArg(testModule, "addRow", func(ctx context.Context) (int, error) {
		data.add()
		return data.count(liveSignedIn(ctx)), nil
	})
	signIn := NewCommand(testModule, "signIn", func(ctx context.Context, user string) (string, error) {
		if err := EventFrom(ctx).SetCookie(liveSessionCookie, user, CookieOptions{}); err != nil {
			return "", err
		}
		return user, identityChanged(ctx)
	})
	signOut := NewCommandNoArg(testModule, "signOut", func(ctx context.Context) (string, error) {
		if err := EventFrom(ctx).DeleteCookie(liveSessionCookie, CookieOptions{}); err != nil {
			return "", err
		}
		return "", identityChanged(ctx)
	})

	app := &liveApp{
		rs:         testRemotes(t, RemoteConfig{}, watchCount, getRows, addRow, signIn, signOut),
		rows:       data,
		frames:     &atomic.Int32{},
		watchCount: watchCount,
		getRows:    getRows,
		addRow:     addRow,
		signIn:     signIn,
		signOut:    signOut,
	}
	app.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.rs.ServeHTTP(&countingWriter{ResponseWriter: w, frames: app.frames}, r)
	}))
	t.Cleanup(app.srv.Close)
	return app
}

// open starts a live-query stream as the given visitor; an empty session is a
// signed-out one.
func (a *liveApp) open(t *testing.T, session string) (*bufio.Reader, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.srv.URL+a.rs.Prefix()+a.watchCount.ID(), nil)
	if err != nil {
		cancel()
		t.Fatalf("building the stream request: %v", err)
	}
	if session != "" {
		req.AddCookie(&http.Cookie{Name: liveSessionCookie, Value: session})
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("opening the stream: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("stream status = %d, want 200", resp.StatusCode)
	}
	return bufio.NewReader(resp.Body), cancel
}

// post invokes a command as the given visitor, asking for the named refreshes,
// and returns the decoded response payload.
func (a *liveApp) post(t *testing.T, fn *Remote, arg any, session string, refreshes ...string) any {
	t.Helper()
	payload := ""
	if arg != nil {
		var err error
		if payload, err = remotearg.StringifyCommandArg(arg); err != nil {
			t.Fatalf("StringifyCommandArg: %v", err)
		}
	}
	if refreshes == nil {
		refreshes = []string{}
	}
	body, err := json.Marshal(map[string]any{"payload": payload, "refreshes": refreshes})
	if err != nil {
		t.Fatalf("marshalling the command body: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, a.srv.URL+a.rs.Prefix()+fn.ID(), strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("building the command request: %v", err)
	}
	if session != "" {
		req.AddCookie(&http.Cookie{Name: liveSessionCookie, Value: session})
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("invoking %s: %v", fn.Name(), err)
	}
	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("reading the command response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s status = %d, want 200 (%s)", fn.Name(), resp.StatusCode, raw)
	}
	kind, data, httpErr := envelope(t, raw)
	if kind != "result" {
		t.Fatalf("%s answered %q: %#v", fn.Name(), kind, httpErr)
	}
	return data
}

// value reads the next result frame and returns the number it carries,
// skipping keep-alive comments.
func liveValue(t *testing.T, br *bufio.Reader) int {
	t.Helper()
	for {
		raw := readFrame(t, br)
		if !strings.HasPrefix(raw, "data: ") {
			continue // a `: keep-alive` comment carries no data
		}
		var frame liveResultFrame
		body := strings.TrimSuffix(strings.TrimPrefix(raw, "data: "), "\n\n")
		if err := json.Unmarshal([]byte(body), &frame); err != nil {
			t.Fatalf("decoding frame %q: %v", raw, err)
		}
		if frame.Type != "result" {
			t.Fatalf("frame is a %q, want a result: %q", frame.Type, raw)
		}
		parsed, err := devalue.Parse(frame.Result, nil)
		if err != nil {
			t.Fatalf("parsing frame payload %q: %v", frame.Result, err)
		}
		n, ok := parsed.(float64)
		if !ok {
			t.Fatalf("frame value is %#v, want a number", parsed)
		}
		return int(n)
	}
}

// quiet asserts that the server has written exactly want frames in total and
// writes no more within a beat. It is how a test says "one value, not three
// racing to it".
func (a *liveApp) quiet(t *testing.T, want int32) {
	t.Helper()
	time.Sleep(300 * time.Millisecond)
	if got := a.frames.Load(); got != want {
		t.Errorf("%d frames written in total, want exactly %d", got, want)
	}
}

// liveKey is the refresh key kit's client sends for a live query with no
// argument: the id, a slash, and an empty payload.
func liveKey(fn *Remote) string { return fn.ID() + "/" }

// bucket returns the `q` or `l` node the client would read a value out of.
func bucket(t *testing.T, data any, which, key string) any {
	t.Helper()
	return field(t, field(t, data, which), key)
}

func TestLiveStreamCountsOnlyWhatTheOpeningRequestMaySee(t *testing.T) {
	app := newLiveApp(t)

	out, cancelOut := app.open(t, "")
	defer cancelOut()
	if got := liveValue(t, out); got != 2 {
		t.Errorf("signed-out stream opened with %d, want 2 — the private row must not be counted", got)
	}

	in, cancelIn := app.open(t, "ada")
	defer cancelIn()
	if got := liveValue(t, in); got != 3 {
		t.Errorf("signed-in stream opened with %d, want 3", got)
	}

	if got := app.rows.watching(); got != 2 {
		t.Errorf("%d open subscriptions, want 2", got)
	}
	app.quiet(t, 2)
}

func TestLiveStreamSendsOneFrameForEachCommand(t *testing.T) {
	app := newLiveApp(t)

	br, cancel := app.open(t, "")
	defer cancel()
	if got := liveValue(t, br); got != 2 {
		t.Fatalf("first frame = %d, want 2", got)
	}

	// Each command adds exactly one row, so the stream must carry exactly one
	// further frame, and it must carry the new total rather than an
	// intermediate one.
	for _, want := range []int{3, 4, 5} {
		app.post(t, app.addRow, nil, "")
		if got := liveValue(t, br); got != want {
			t.Fatalf("frame after a command = %d, want %d", got, want)
		}
	}
	app.quiet(t, 4)
}

func TestOpenLiveStreamKeepsTheIdentityItOpenedWith(t *testing.T) {
	app := newLiveApp(t)

	br, cancel := app.open(t, "")
	defer cancel()
	if got := liveValue(t, br); got != 2 {
		t.Fatalf("first frame = %d, want 2", got)
	}

	// Signing in cannot reach a stream that is already open: kit's event is a
	// snapshot of the request that opened it. The command answers with a seed
	// for the *new* identity and the client reconnects.
	data := app.post(t, app.signIn, "ada", "", liveKey(app.watchCount))
	if got := field(t, bucket(t, data, "l", liveKey(app.watchCount)), "v"); got != float64(3) {
		t.Errorf("seed in `l` = %#v, want 3 — it must be computed under the cookie the command just wrote", got)
	}

	// The already-open stream must not have been disturbed, and must still be
	// answering as the signed-out visitor it opened as.
	app.quiet(t, 1)
	app.post(t, app.addRow, nil, "ada")
	if got := liveValue(t, br); got != 3 {
		t.Errorf("the open stream reported %d after a row was added, want 3 — it must keep its signed-out identity, not adopt the new cookie", got)
	}
	app.quiet(t, 2)
}

func TestReconnectedLiveStreamAgreesWithTheSeedExactlyOnce(t *testing.T) {
	app := newLiveApp(t)

	br, cancel := app.open(t, "")
	if got := liveValue(t, br); got != 2 {
		t.Fatalf("first frame = %d, want 2", got)
	}

	data := app.post(t, app.signIn, "ada", "", liveKey(app.watchCount))
	seed, ok := field(t, bucket(t, data, "l", liveKey(app.watchCount)), "v").(float64)
	if !ok {
		t.Fatalf("seed is not a number")
	}

	// Kit's client seeds the value from `l` and then tears the stream down and
	// opens a new one carrying the cookie the command wrote. The reconnected
	// stream must open on the value the client is already showing — a
	// different first frame would make the number flicker, and no first frame
	// at all would strand it.
	cancel()
	reconnected, cancelAgain := app.open(t, "ada")
	defer cancelAgain()

	if got := liveValue(t, reconnected); float64(got) != seed {
		t.Errorf("reconnected stream opened with %d, want the seeded %v", got, seed)
	}
	// Two frames from the first stream's lifetime and one from the second.
	app.quiet(t, 2)

	// Signing out again seeds the signed-out count from the same jar.
	data = app.post(t, app.signOut, nil, "ada", liveKey(app.watchCount))
	if got := field(t, bucket(t, data, "l", liveKey(app.watchCount)), "v"); got != float64(2) {
		t.Errorf("seed after signing out = %#v, want 2", got)
	}
}

func TestSeedingALiveQueryTearsDownTheProducerItStarted(t *testing.T) {
	app := newLiveApp(t)

	app.post(t, app.signIn, "ada", "", liveKey(app.watchCount))

	// Taking one value must start and stop exactly one producer. A seed that
	// leaked its subscription would keep the store alive for the life of the
	// process and would be counted by every later broadcast.
	opened, closed := app.rows.lifecycle()
	if opened != 1 || closed != 1 {
		t.Errorf("producer opened %d times and closed %d, want 1 and 1", opened, closed)
	}
	if got := app.rows.watching(); got != 0 {
		t.Errorf("%d subscriptions still open after seeding, want 0", got)
	}
}

func TestRefreshSeparatesLiveQueriesFromQueries(t *testing.T) {
	app := newLiveApp(t)

	data := app.post(t, app.signIn, "ada", "",
		liveKey(app.watchCount),
		app.getRows.ID()+"/",
		app.addRow.ID()+"/", // a command is not refreshable
		"nosuchhash/watchCount/",
		"no-slash-at-all",
	)

	// The client reads `q` and `l` differently: a `q` entry replaces a query's
	// value, an `l` entry seeds a live query and then reconnects its stream.
	// Putting a live query in `q` would leave the stream on the old identity.
	q := object(t, field(t, data, "q"))
	if q.Len() != 1 {
		t.Errorf("q holds %v, want only the plain query", q.Keys())
	}
	l := object(t, field(t, data, "l"))
	if l.Len() != 1 {
		t.Errorf("l holds %v, want only the live query", l.Keys())
	}
	if _, present := l.Get(app.getRows.ID() + "/"); present {
		t.Error("the plain query landed in `l`")
	}
	if _, present := q.Get(liveKey(app.watchCount)); present {
		t.Error("the live query landed in `q`; its stream would never reconnect")
	}
	if got := field(t, bucket(t, data, "q", app.getRows.ID()+"/"), "v"); got != float64(3) {
		t.Errorf("refreshed query = %#v, want 3", got)
	}
	if got := field(t, data, "r"); got != true {
		t.Errorf("r = %#v, want true", got)
	}
}

func TestSeedingALiveQueryThatYieldsNothingReportsAnError(t *testing.T) {
	// Kit reconnects a live query only when its `l` node carries a value; a
	// node carrying an error is terminal, so a producer that yields nothing
	// has to be reported as one rather than as an empty success.
	silentFn := func(ctx context.Context, yield func(int) error) error {
		return nil
	}
	silent := NewLiveQueryNoArg(testModule, "watchCount", silentFn)
	cmd := NewCommandNoArg(testModule, "signIn", func(ctx context.Context) (string, error) {
		return "ok", ReconnectRequestedNoArg(ctx, silentFn)
	})
	rs := testRemotes(t, RemoteConfig{}, silent, cmd)

	body, err := json.Marshal(map[string]any{"payload": "", "refreshes": []string{liveKey(silent)}})
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, rs.Prefix()+cmd.ID(), strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, req)

	_, data, _ := envelope(t, rec.Body.Bytes())
	entry := field(t, field(t, data, "l"), liveKey(silent))
	if _, present := object(t, entry).Get("v"); present {
		t.Error("a live query that yielded nothing reported a value")
	}
	if got := field(t, field(t, entry, "e"), "status"); got != float64(500) {
		t.Errorf("error status = %#v, want 500", got)
	}
}
