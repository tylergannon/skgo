package ssr

import (
	"fmt"
	"log"
)

// Console is told what the engine wrote to `console`. level is the method that
// was called — "log", "warn", "error" and the rest — routeID is the route being
// rendered, or "" for something written while the bundle was being evaluated,
// and text is the arguments joined the way a browser's console would show them.
//
// It exists because the engine is a bare ECMAScript runtime: kit and Svelte
// both report through `console`, and with no such global those reports were a
// ReferenceError thrown in the middle of a render rather than a line an
// operator could read.
type Console func(routeID, level, text string)

// defaultConsole is where the engine's console goes when the caller names
// nowhere else. Losing it is not an option: it is the only channel a render
// error inside the engine has.
func defaultConsole(routeID, level, text string) {
	if routeID == "" {
		log.Printf("skgo: console.%s: %s", level, text)
		return
	}
	log.Printf("skgo: console.%s rendering %s: %s", level, routeID, text)
}

// installGlobals gives a fresh runtime the globals kit's and Svelte's runtime
// code assume and goja does not have.
//
// It runs before the bundle is evaluated, so the bundle's own top-level code
// gets them too — which is the point of `console` in particular: a bundle that
// fails while it is coming up now says why.
func (rt *runtime) installGlobals(console Console) error {
	if err := rt.vm.Set("__skgo_console", func(level, text string) {
		console(rt.route, level, text)
	}); err != nil {
		return err
	}
	if _, err := rt.vm.RunString(globalsSource); err != nil {
		return fmt.Errorf("skgo: installing the engine's globals: %w", err)
	}
	return nil
}

// globalsSource is the three globals, in the engine's own language.
//
//   - `Symbol.asyncIterator`, which kit's live-query iterators declare as a
//     computed method name and its `to_iterator` tests for. Without it every
//     such object grows a property literally called "undefined" and the test
//     finds the wrong thing rather than failing.
//   - `Promise.withResolvers`, which kit's `create_async_iterator` calls
//     (`utils/streaming.js`).
//   - `console`, which kit's `log_handle_error_hook_failure` and Svelte's
//     `unresolved_hydratable` call unconditionally, in a production build, on
//     paths a render actually reaches.
//   - `setTimeout`, which is a queue and not a timer. Kit's `query.batch`
//     collects the calls a render made and flushes them with
//     `setTimeout(..., 0)`, deliberately a macrotask so that everything awaited
//     in the same turn ends up in one batch
//     (`runtime/app/server/remote/query.js`). goja has no job queue beyond
//     promises, so this one records the callbacks and Render drains them
//     between microtask turns. It never waits: the delay is read only to keep
//     the queue in the order a real one would run it, and a callback scheduled
//     for later runs immediately after the ones scheduled for sooner. Nothing
//     here sleeps, so the rule the engine obeys — no I/O, no clock — still
//     holds.
//
// The formatting is done here rather than in Go so that an Error arrives as its
// stack and an object as JSON, which is what makes the line worth reading.
const globalsSource = `
(function (report) {
	if (typeof Symbol.asyncIterator === 'undefined') {
		Object.defineProperty(Symbol, 'asyncIterator', {
			value: Symbol('Symbol.asyncIterator')
		});
	}

	if (typeof Promise.withResolvers !== 'function') {
		Promise.withResolvers = function withResolvers() {
			var resolve, reject;
			var promise = new this(function (res, rej) {
				resolve = res;
				reject = rej;
			});
			return { promise: promise, resolve: resolve, reject: reject };
		};
	}

	// The macrotask queue. Render drains it after each turn of the microtask
	// queue, which is exactly when a real setTimeout(fn, 0) would run.
	var queue = [];
	var next_id = 1;
	globalThis.setTimeout = function (fn, delay) {
		if (typeof fn !== 'function') return 0;
		var args = Array.prototype.slice.call(arguments, 2);
		var id = next_id++;
		queue.push({ id: id, fn: fn, args: args, delay: delay > 0 ? delay : 0 });
		return id;
	};
	globalThis.clearTimeout = function (id) {
		for (var i = 0; i < queue.length; i++) {
			if (queue[i].id === id) {
				queue.splice(i, 1);
				return;
			}
		}
	};
	globalThis.setInterval = function () {
		throw new Error('skgo: setInterval is not available during server-side rendering');
	};
	globalThis.clearInterval = globalThis.clearTimeout;

	// How many callbacks are waiting. Go reads this to decide whether a render
	// that has not finished still has somewhere to go.
	globalThis.__skgo_pending = function () {
		return queue.length;
	};

	// Empties the queue. Go calls this as a render starts, because a runtime is
	// pooled and reused: a callback the last render scheduled and never needed
	// belongs to a request that is over, and running it inside the next render
	// would be one visitor's work charged to another. It happens — a component
	// that reads .loading on a batch query without awaiting it schedules a
	// flush the render never waits for.
	globalThis.__skgo_reset = function () {
		queue.length = 0;
	};

	// Runs the callback that a real event loop would run next: the one with the
	// shortest delay, and among equals the one scheduled first. Returns whether
	// anything ran. Go calls this from the outside, so the promise jobs the
	// callback queues are drained when it returns.
	globalThis.__skgo_tick = function () {
		if (queue.length === 0) return false;
		var at = 0;
		for (var i = 1; i < queue.length; i++) {
			if (queue[i].delay < queue[at].delay) at = i;
		}
		var task = queue.splice(at, 1)[0];
		task.fn.apply(undefined, task.args);
		return true;
	};

	function show(value) {
		if (typeof value === 'string') return value;
		if (value instanceof Error) return value.stack || (value.name + ': ' + value.message);
		if (value === undefined) return 'undefined';
		if (value === null) return 'null';
		try {
			var json = JSON.stringify(value);
			if (json !== undefined) return json;
		} catch (e) {}
		try {
			return String(value);
		} catch (e) {
			return '[unprintable]';
		}
	}

	var out = {};
	['log', 'info', 'warn', 'error', 'debug', 'trace', 'dir'].forEach(function (level) {
		out[level] = function () {
			var parts = [];
			for (var i = 0; i < arguments.length; i++) parts.push(show(arguments[i]));
			report(level, parts.join(' '));
		};
	});
	globalThis.console = out;
})(__skgo_console);
`
