import { mkdtempSync, mkdirSync, readFileSync, realpathSync, rmSync } from 'node:fs';
import { createHash, randomUUID } from 'node:crypto';
import { spawn } from 'node:child_process';
import { createRequire } from 'node:module';
import process from 'node:process';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { isMainThread, parentPort } from 'node:worker_threads';

const TAG = 'skgo-prerender-v1';
const owners = new Map();
const requestWaiters = new Map();
const ownerByRoot = new Map();
let parentPortListening = false;

function appRoot() {
	return resolve(process.cwd());
}

function kitRootFor(root) {
	return realpathSync(join(root, 'node_modules/@sveltejs/kit'));
}

function kitQueueFailure(detail) {
	const error = makeError(`SKGO_KIT_PRERENDER_QUEUE: ${detail}. Run "go tool skgo kit-patch --web web --apply" from the Go app root, then "cd web && node_modules/.bin/vp install --no-frozen-lockfile", then "go tool skgo kit-patch --web web --check".`);
	error.name = 'SKGO_KIT_PRERENDER_QUEUE';
	error.code = 'SKGO_KIT_PRERENDER_QUEUE';
	return error;
}

function verifyKitQueue(root) {
	let metadata;
	try {
		const appRequire = createRequire(join(root, 'package.json'));
		const adapterPackage = appRequire.resolve('@skgo/sveltekit-adapter/package.json');
		metadata = JSON.parse(readFileSync(join(dirname(adapterPackage), 'skgo-adapter/compat/kit-3.0.0-queue.json'), 'utf8'));
	} catch (error) {
		throw kitQueueFailure(`adapter compatibility metadata could not be read: ${error.message}`);
	}
	try {
		const kitRoot = kitRootFor(root);
		const pkg = JSON.parse(readFileSync(join(kitRoot, 'package.json'), 'utf8'));
		const queuePath = join(kitRoot, metadata.queuePath);
		const queue = readFileSync(queuePath);
		const digest = createHash('sha256').update(queue).digest('hex');
		if (pkg.name !== metadata.package || pkg.version !== metadata.version || digest !== metadata.correctedSHA256) {
			throw kitQueueFailure(`resolved ${pkg.name ?? 'unknown'}@${pkg.version ?? 'unknown'} queue SHA-256 ${digest}; expected ${metadata.package}@${metadata.version} with SHA-256 ${metadata.correctedSHA256}`);
		}
	} catch (error) {
		if (error?.code === 'SKGO_KIT_PRERENDER_QUEUE') throw error;
		throw kitQueueFailure(`resolved Kit queue could not be verified: ${error.message}`);
	}
}

function generatedConfig(root) {
	const manifest = JSON.parse(readFileSync(join(root, 'skgo.remotes.json'), 'utf8'));
	if (!manifest.prerender?.root || !manifest.prerender?.package) {
		throw new Error('skgo: generated prerender command is missing');
	}
	return {
		cwd: resolve(root, manifest.prerender.root),
		pkg: manifest.prerender.package
	};
}

function vitePlusBuildParent(root) {
	if (!isMainThread || process.env.NODE_PACKAGE_MANAGER !== 'vite-plus' || process.argv[2] !== 'build' || !process.argv[1]) return null;
	try {
		const appRequire = createRequire(join(root, 'package.json'));
		const vpRequire = createRequire(appRequire.resolve('vite-plus/package.json'));
		const expected = realpathSync(join(dirname(vpRequire.resolve('vite')), 'cli.js'));
		return realpathSync(process.argv[1]) === expected && process.ppid > 1 ? process.ppid : null;
	} catch {
		return null;
}
}

function makeError(message) {
	const error = new Error(message);
	return error;
}

function reportOwnerFailure(error) {
	const primary = error?.message ?? String(error);
	const cleanup = error?.cleanupError ? `; cleanup failed: ${error.cleanupError.message ?? error.cleanupError}` : '';
	try { process.stderr.write(`skgo prerender owner failure: ${primary}${cleanup}\n`); } catch {}
}

function validateEnvelope(request, raw) {
	let value;
	try { value = JSON.parse(raw); }
	catch (error) { throw makeError(`skgo prerender command returned malformed JSON: ${error.message}`); }
	if (!value || typeof value !== 'object' || Array.isArray(value)) throw makeError('skgo prerender command returned a non-object response');
	if (request.kind === 'remote-inputs') {
		if (typeof value.inputs !== 'string') throw makeError('skgo prerender inputs response has no devalue payload');
	} else if (request.kind === 'remote') {
		if (value.type === 'result' && typeof value.data === 'string') return;
		if (value.type === 'error' && value.kind === 'app' && value.error && Number.isInteger(value.error.status) && typeof value.error.message === 'string') return;
		if (value.type === 'error' && value.kind === 'unknown' && value.error && Number.isInteger(value.error.status) && typeof value.error.message === 'string' && typeof value.diagnostic === 'string') return;
		if (value.type === 'redirect' && Number.isInteger(value.redirect?.status) && value.redirect.status >= 300 && value.redirect.status <= 308 && typeof value.redirect.location === 'string') return;
		throw makeError('skgo prerender remote response has an invalid envelope');
	} else if (request.kind === 'load') {
		const allowed = ['data', 'chunks', 'error', 'failure', 'redirect', 'headers', 'cookies'];
		if (!allowed.some((key) => key in value)) throw makeError('skgo prerender load response has no recognized fields');
		if (value.data !== undefined && typeof value.data !== 'string' && !(value.data && typeof value.data === 'object')) throw makeError('skgo prerender load response has invalid data');
		if (value.failure !== undefined && typeof value.failure !== 'string') throw makeError('skgo prerender load response has invalid failure');
	} else {
		throw makeError(`skgo prerender owner received unsupported request kind ${request.kind}`);
	}
}

function groupAlive(job) {
	if (process.platform === 'win32' || !job.group) return false;
	try {
		process.kill(-job.group, 0);
		return true;
	} catch (error) {
		if (error?.code === 'ESRCH') return false;
		return true;
	}
}

function signalTree(job, signal) {
	if (process.platform === 'win32') {
		if (!job.pid) return;
		try {
			const killer = spawn('taskkill', ['/PID', String(job.pid), '/T', '/F'], {
				stdio: 'ignore',
				windowsHide: true
			});
			job.killers.add(killer);
			killer.once('close', () => job.killers.delete(killer));
			killer.once('error', () => job.killers.delete(killer));
		} catch {
			// The direct child is still retained and will be awaited below.
		}
		return;
	}
	if (!job.group) return;
	try {
		process.kill(-job.group, signal);
	} catch (error) {
		if (error?.code !== 'ESRCH') throw error;
	}
}

async function stopGroup(job, graceMs = 150) {
	if (process.platform === 'win32') {
		signalTree(job, 'SIGKILL');
		await Promise.allSettled([...job.killers].map((killer) => new Promise((resolve) => killer.once('close', resolve))));
		return;
	}
	if (!groupAlive(job)) return;
	signalTree(job, 'SIGTERM');
	await new Promise((resolve) => setTimeout(resolve, graceMs));
	if (groupAlive(job)) signalTree(job, 'SIGKILL');
	const deadline = Date.now() + 5000;
	while (groupAlive(job) && Date.now() < deadline) {
		await new Promise((resolve) => setTimeout(resolve, 20));
	}
	if (groupAlive(job)) throw makeError(`skgo could not drain process group ${job.group}`);
}

function beginTermination(job) {
	try { signalTree(job, 'SIGTERM'); } catch {}
	if (process.platform === 'win32') return Promise.resolve();
	return new Promise((resolve) => setTimeout(() => {
		if (groupAlive(job)) {
			try { signalTree(job, 'SIGKILL'); } catch {}
		}
		resolve();
	}, 150));
}

function startDrain(job) {
	if (!job.drainPromise) job.drainPromise = beginTermination(job);
	return job.drainPromise;
}

function createJob(session, command, args, options = {}) {
	const detached = process.platform !== 'win32';
	const child = spawn(command, args, {
		cwd: options.cwd,
		env: options.env ?? process.env,
		stdio: options.input === undefined ? ['ignore', 'pipe', 'pipe'] : ['pipe', 'pipe', 'pipe'],
		detached,
		windowsHide: true
	});
	const job = {
		child,
		pid: child.pid,
		group: detached ? child.pid : null,
		killers: new Set(),
		stdout: '',
		stderr: '',
		closed: false,
		settled: false,
		closePromise: null,
		resolveClose: null,
		drainPromise: null,
		done: null,
		resolve: null,
		reject: null
	};
	session.jobs.add(job);
	session.groups.add(job);
	job.done = new Promise((resolve, reject) => {
		job.resolve = resolve;
		job.reject = reject;
	});
	job.closePromise = new Promise((resolve) => { job.resolveClose = resolve; });
	job.done.catch(() => {});
	const settle = (error, value) => {
		if (job.settled) return;
		job.settled = true;
		if (error) job.reject(error);
		else job.resolve(value);
	};
	const pipeError = (label) => (error) => {
		settle(makeError(`skgo prerender ${label} pipe failed: ${error.message}`));
		signalTree(job, 'SIGTERM');
	};
	child.on('error', (error) => {
		settle(makeError(`skgo prerender process failed to start: ${error.message}`));
	});
	// Do not wait for inherited stdout/stderr pipes to close before starting
	// descendant termination: a child which inherited either pipe can hold the
	// leader's `close` event open after the Go process itself has exited.
	child.on('exit', () => { void startDrain(job); });
	child.stdout.setEncoding('utf8');
	child.stderr.setEncoding('utf8');
	child.stdout.on('data', (chunk) => { job.stdout += chunk; });
	child.stderr.on('data', (chunk) => { job.stderr += chunk; });
	child.stdin?.on('error', pipeError('stdin'));
	child.stdout.on('error', pipeError('stdout'));
	child.stderr.on('error', pipeError('stderr'));
	child.stdin?.on('close', () => {
		if (options.input !== undefined && !job.closed && !child.stdin.writableFinished) {
			settle(makeError('skgo prerender process closed stdin before accepting its request'));
			signalTree(job, 'SIGTERM');
		}
	});
	child.on('close', async (code, signal) => {
		job.closed = true;
		const exit = { code, signal, stdout: job.stdout, stderr: job.stderr };
		let treeError;
		// A successful leader can leave compiler or application descendants in
		// its process group. Drain them before any request or outer build settles.
		await startDrain(job);
		await stopGroup(job).catch((error) => { treeError = error; });
		session.jobs.delete(job);
		if (!treeError) session.groups.delete(job);
		job.resolveClose();
		if (treeError) {
			settle(treeError);
		} else if (!job.settled) {
			if (code !== 0) {
				settle(makeError(`skgo prerender command exited ${code ?? signal}: ${job.stderr.trim()}`));
			} else if (options.request) {
				try {
					validateEnvelope(options.request, job.stdout);
					settle(null, exit);
				} catch (error) {
					settle(makeError(`${error.message}; ${job.stderr.trim()}`));
				}
			} else {
				settle(null, exit);
			}
		}
	});
	if (options.input !== undefined) {
		// All child and pipe listeners exist before writing the request.
		child.stdin.end(options.input);
	}
	return job;
}

function makeSession(root, ownerParentPid) {
	const session = {
		id: randomUUID(),
		root,
		cwd: null,
		pkg: null,
		dir: null,
		binary: null,
		compilePromise: null,
		jobs: new Set(),
		groups: new Set(),
		workers: new Map(),
		observedWorkers: new Map(),
		closing: false,
		failed: null,
		cleanupPromise: null,
		ownerParentPid,
		parentObserver: null,
		parentLossStarted: false,
		signalHandlers: new Map(),
		watchParent() {
			if (session.ownerParentPid === null || session.parentObserver) return;
			session.parentObserver = setInterval(() => {
				if (process.ppid === session.ownerParentPid && process.ppid > 1) return;
				clearInterval(session.parentObserver);
				session.parentObserver = null;
				if (session.parentLossStarted) return;
				session.parentLossStarted = true;
				void (async () => {
					try {
						await session.fail(makeError('skgo prerender Vite build parent exited'));
						for (const [signal, handler] of session.signalHandlers) process.off(signal, handler);
						session.signalHandlers.clear();
						process.exitCode = 1;
						try { process.kill(process.pid, 'SIGTERM'); }
						catch { process.exit(1); }
					} catch (error) {
						process.exitCode = 1;
						reportOwnerFailure(error);
					}
				})();
			}, 50);
			session.parentObserver.unref();
		},
		fail(error) {
			if (!session.failed) session.failed = error;
			session.closing = true;
			return session.cleanup().catch((cleanupError) => {
				session.cleanupFailure = cleanupError;
				if (session.failed && typeof session.failed === 'object') session.failed.cleanupError = cleanupError;
				throw session.failed ?? cleanupError;
			});
		},
		async ensureCompiled() {
			if (session.closing) throw session.failed ?? makeError('skgo prerender owner is closed');
			if (!session.compilePromise) {
				const config = generatedConfig(root);
				session.cwd = config.cwd;
				session.pkg = config.pkg;
				session.dir = mkdtempSync(join(tmpdir(), 'skgo-prerender-'));
				session.binary = join(session.dir, process.platform === 'win32' ? 'build.exe' : 'build');
				const onExit = () => {
					for (const job of session.groups) signalTree(job, 'SIGKILL');
					try { rmSync(session.dir, { recursive: true, force: true }); } catch {}
				};
				process.once('exit', onExit);
				session.exitHandler = onExit;
				for (const signal of ['SIGINT', 'SIGTERM']) {
					const handler = async () => {
						try {
							await session.fail(makeError(`skgo prerender build interrupted by ${signal}`));
							process.off(signal, handler);
							try { process.kill(process.pid, signal); } catch { process.exitCode = 1; }
						} catch (error) {
							process.exitCode = 1;
							reportOwnerFailure(error);
							process.once(signal, handler);
						}
					};
					session.signalHandlers.set(signal, handler);
					process.once(signal, handler);
				}
				mkdirSync(join(session.dir, 'go-tmp'), { recursive: true });
				session.compilePromise = (async () => {
					try {
						const job = createJob(session, 'go', ['build', '-o', session.binary, session.pkg], {
							cwd: session.cwd,
						env: { ...process.env, GOTMPDIR: join(session.dir, 'go-tmp') }
						});
						const result = await job.done;
						if (result.code !== 0) throw makeError(`skgo prerender compiler failed: ${result.stderr.trim()}`);
					} catch (error) {
						session.failed ??= error;
						session.closing = true;
						await session.cleanup();
						throw error;
					}
				})();
			}
			await session.compilePromise;
			if (session.closing) throw session.failed ?? makeError('skgo prerender owner is closed');
		},
		async invoke(request) {
			await session.ensureCompiled();
			if (session.closing) throw session.failed ?? makeError('skgo prerender owner is closed');
			const job = createJob(session, session.binary, [], {
				cwd: session.cwd,
				input: JSON.stringify(request),
				request
			});
			try {
				const result = await job.done;
				if (session.closing) throw session.failed ?? makeError('skgo prerender owner is closed');
				return result.stdout;
			} catch (error) {
				await session.fail(error);
				throw error;
			}
		},
		async cleanup() {
			if (session.cleanupPromise) return session.cleanupPromise;
			session.closing = true;
			const cleanupPromise = (async () => {
				const jobs = [...session.jobs];
				const escalations = jobs.map(startDrain);
				// Start TERM-to-KILL escalation before waiting for children to close.
				await Promise.allSettled([...jobs.map((job) => job.closePromise), ...escalations]);
				const drainErrors = [];
				for (const job of [...session.groups]) {
					try {
						await stopGroup(job);
						session.groups.delete(job);
					} catch (error) {
						drainErrors.push(error);
					}
				}
				if (drainErrors.length) {
					const error = new AggregateError(drainErrors, 'skgo could not drain every owned prerender process tree');
					session.cleanupFailure = error;
					if (session.failed && typeof session.failed === 'object') session.failed.cleanupError = error;
					throw error;
				}
				for (const workerRecord of session.workers.values()) {
					for (const requestId of workerRecord.handshakes) {
						try { workerRecord.worker.postMessage({ tag: TAG, type: 'failed', appRoot: root, session: session.id, requestId, error: session.failed?.message ?? 'owner closed' }); } catch {}
					}
					workerRecord.handshakes.clear();
					for (const requestId of workerRecord.pending.keys()) {
						try { workerRecord.worker.postMessage({ tag: TAG, type: 'failed', appRoot: root, session: session.id, requestId, error: session.failed?.message ?? 'owner closed' }); } catch {}
					}
					workerRecord.pending.clear();
					workerRecord.worker.off('message', workerRecord.onMessage);
					workerRecord.worker.off('error', workerRecord.onError);
					workerRecord.worker.off('exit', workerRecord.onExit);
				}
				for (const [worker, listener] of session.observedWorkers) worker.off('message', listener);
				session.observedWorkers.clear();
				session.workers.clear();
				if (session.dir) rmSync(session.dir, { recursive: true, force: true });
				if (session.exitHandler) process.off('exit', session.exitHandler);
				if (session.parentObserver) clearInterval(session.parentObserver);
				session.parentObserver = null;
				for (const [signal, handler] of session.signalHandlers) process.off(signal, handler);
				process.off('worker', session.onWorker);
				session.jobs.clear();
				session.groups.clear();
				ownerByRoot.delete(root);
			})();
			session.cleanupPromise = cleanupPromise;
			try {
				await cleanupPromise;
			} catch (error) {
				// Retain listeners, directories and failed group records so later
				// cleanup attempts can retry rather than forgetting live ownership.
				session.cleanupPromise = null;
				throw error;
			}
		}
	};
	return session;
}

function response(worker, message, fields) {
	try { worker.postMessage({ tag: TAG, appRoot: message.appRoot, session: message.session, requestId: message.requestId, ...fields }); } catch {}
}

async function connectWorker(session, worker, message) {
	if (session.closing) {
		await session.cleanup();
		response(worker, message, { type: 'failed', error: session.failed?.message ?? 'owner is closed' });
		return;
	}
	let record = session.workers.get(worker);
	if (!record) {
		record = { worker, pending: new Map(), handshakes: new Set(), exited: false };
		session.workers.set(worker, record);
		record.onMessage = (next) => {
			if (!next || next.tag !== TAG || next.appRoot !== session.root || next.session !== session.id) return;
			if (next.type === 'connected-ack') {
				record.handshakes.delete(next.requestId);
				return;
			}
			void handleWorkerRequest(session, record, next).catch(reportOwnerFailure);
		};
		record.onError = (error) => { void session.fail(error).catch(reportOwnerFailure); };
		record.onExit = (code) => {
			record.exited = true;
			if (record.pending.size > 0 || record.handshakes.size > 0 || code !== 0) {
				session.closing = true;
				void session.fail(makeError(`Kit prerender worker exited ${code} with ${record.pending.size} pending Go requests and ${record.handshakes.size} pending handshakes`)).catch(reportOwnerFailure);
			}
		};
		worker.on('message', record.onMessage);
		worker.on('error', record.onError);
		worker.on('exit', record.onExit);
	}
	record.handshakes.add(message.requestId);
	try { worker.postMessage({ tag: TAG, type: 'connected', appRoot: session.root, session: session.id, requestId: message.requestId }); }
	catch (error) {
		record.handshakes.delete(message.requestId);
		await session.fail(error);
	}
}

async function handleWorkerRequest(session, record, message) {
	if (record.exited) return;
	if (session.closing) {
		await session.cleanup();
		response(record.worker, message, { type: 'failed', error: session.failed?.message ?? 'owner is closed' });
		return;
	}
	if (message.type === 'fatal') {
		record.pending.set(message.requestId, { worker: record.worker, requestId: message.requestId });
		await session.fail(makeError(`Kit prerender worker reported a fatal protocol error: ${message.error ?? 'unknown error'}`));
		return;
	}
	if (message.type !== 'invoke') {
		response(record.worker, message, { type: 'failed', error: session.failed?.message ?? 'owner is closed' });
		return;
	}
	const pending = { worker: record.worker, requestId: message.requestId };
	record.pending.set(message.requestId, pending);
	try {
		const raw = await session.invoke(message.request);
		if (record.exited || session.closing) throw session.failed ?? makeError('owner closed while request was running');
		record.pending.delete(message.requestId);
		response(record.worker, message, { type: 'result', raw });
	} catch (error) {
		await session.fail(error);
		record.pending.delete(message.requestId);
		response(record.worker, message, { type: 'failed', error: error.message });
	}
}

/** Create the process owner from the main Vite build thread. */
export function createPrerenderOwner(root = appRoot()) {
	if (!isMainThread) return null;
	root = resolve(root);
	const existing = ownerByRoot.get(root);
	if (existing) return existing;
	const session = makeSession(root, vitePlusBuildParent(root));
	ownerByRoot.set(root, session);
	session.watchParent();
	session.onWorker = (worker) => {
		const listener = (message) => {
			if (!message || message.tag !== TAG || message.type !== 'connect' || message.appRoot !== root) return;
			if (message.session !== undefined && message.session !== session.id) return;
			void connectWorker(session, worker, message).catch(reportOwnerFailure);
		};
		session.observedWorkers.set(worker, listener);
		worker.on('message', listener);
	};
	process.on('worker', session.onWorker);
	return session;
}

/** Wrap Vite's whole native app-build boundary so its caller cannot reject before Go drains. */
export function installPrerenderFailureBoundary(config, owner) {
	if (!owner || typeof config?.getSortedPlugins !== 'function') return;
	const boundary = owner.boundary ??= {
		seenPlugins: new WeakSet(),
		wrappedHandlers: new WeakMap(),
		wrappers: new WeakSet()
	};
	const wrap = (handler) => {
		if (boundary.wrappers.has(handler)) return handler;
		const existing = boundary.wrappedHandlers.get(handler);
		if (existing) return existing;
		const wrapped = async function (...args) {
			let result;
			try {
				result = await handler.apply(this, args);
			} catch (error) {
				try {
					await owner.fail(error);
				} catch (failure) {
					const cleanupFailure = owner.cleanupFailure ?? failure?.cleanupError ?? failure;
					if (error && typeof error === 'object') {
						try { error.cleanupError = cleanupFailure; }
						catch { reportOwnerFailure({ message: 'could not attach prerender cleanup failure to the native build error', cleanupError: cleanupFailure }); }
					} else {
						reportOwnerFailure({ message: `native build error: ${String(error)}`, cleanupError: cleanupFailure });
					}
				}
				throw error;
			}
			if (owner.failed) {
				try { await owner.cleanup(); }
				catch (cleanupError) {
					if (typeof owner.failed === 'object') owner.failed.cleanupError = cleanupError;
				}
				throw owner.failed;
			}
			return result;
		};
		boundary.wrappedHandlers.set(handler, wrapped);
		boundary.wrappers.add(wrapped);
		return wrapped;
	};
	const setHook = (target, replacement) => {
		const descriptor = Object.getOwnPropertyDescriptor(target, 'buildApp') ?? {
			configurable: true, enumerable: true, writable: true
		};
		Object.defineProperty(target, 'buildApp', { ...descriptor, value: replacement });
	};
	for (const plugin of config.getSortedPlugins('buildApp')) {
		if (!plugin || typeof plugin !== 'object' || boundary.seenPlugins.has(plugin)) continue;
		boundary.seenPlugins.add(plugin);
		const hook = plugin.buildApp;
		if (typeof hook === 'function') {
			setHook(plugin, wrap(hook));
		} else if (hook && typeof hook.handler === 'function') {
			const descriptors = Object.getOwnPropertyDescriptors(hook);
			descriptors.handler = { ...descriptors.handler, value: wrap(hook.handler) };
			setHook(plugin, Object.create(Object.getPrototypeOf(hook), descriptors));
		}
	}
	const builder = config.builder;
	if (builder && typeof builder === 'object' && typeof builder.buildApp === 'function') {
		setHook(builder, wrap(builder.buildApp));
	}
}

/** Wait for cleanup only when a native failure or signal has begun draining. */
export async function joinFailedPrerenderOwner(owner) {
	if (owner?.closing) {
		try { await owner.cleanup(); }
		catch (cleanupError) {
			if (owner.failed && typeof owner.failed === 'object') owner.failed.cleanupError = cleanupError;
			throw owner.failed ?? cleanupError;
		}
	}
}

function receiveWorkerMessage(message) {
	if (!message || message.tag !== TAG || !message.requestId) return;
	const waiter = requestWaiters.get(message.requestId);
	if (!waiter) return;
	if (message.type === 'connected') waiter.resolve(message.session);
	else if (message.type === 'result') waiter.resolve(message.raw);
	else if (message.type === 'failed') waiter.reject(makeError(message.error ?? 'prerender owner failed'));
	else return;
	requestWaiters.delete(message.requestId);
}

function ensureWorkerListener() {
	if (parentPortListening || !parentPort) return;
	parentPortListening = true;
	parentPort.on('message', receiveWorkerMessage);
}

function workerRequest(message) {
	if (!parentPort || isMainThread) return Promise.reject(makeError('prerender callbacks require a registered Kit worker'));
	ensureWorkerListener();
	return new Promise((resolve, reject) => {
		const timer = message.type === 'connect' ? setTimeout(() => {
			requestWaiters.delete(message.requestId);
			reject(makeError('skgo prerender owner is unavailable or did not answer the worker handshake'));
		}, 5000) : null;
		requestWaiters.set(message.requestId, {
			resolve: (value) => { if (timer) clearTimeout(timer); resolve(value); },
			reject: (error) => { if (timer) clearTimeout(timer); reject(error); }
		});
		try { parentPort.postMessage({ tag: TAG, ...message }); }
		catch (error) {
			if (timer) clearTimeout(timer);
			requestWaiters.delete(message.requestId);
			reject(error);
		}
	});
}

async function workerSession(root) {
	const requestId = randomUUID();
	const session = await workerRequest({ type: 'connect', appRoot: root, requestId });
	if (typeof session !== 'string' || !session) throw makeError('skgo prerender owner handshake returned no session');
	parentPort.postMessage({ tag: TAG, type: 'connected-ack', appRoot: root, session, requestId });
	return session;
}

async function invokeWorker(request) {
	const root = appRoot();
	const session = await workerSession(root);
	const requestId = randomUUID();
	return workerRequest({ type: 'invoke', appRoot: root, session, requestId, request });
}

async function reportWorkerFatal(error) {
	const root = appRoot();
	let session;
	try { session = await workerSession(root); }
	catch { throw error; }
	const requestId = randomUUID();
	try {
		await workerRequest({
			type: 'fatal', appRoot: root, session, requestId,
			error: error instanceof Error ? error.message : String(error)
		});
	} catch {
		// The owner replies only after it has drained. Preserve the original
		// worker-side failure as the exception Kit sees.
	}
	throw error;
}

async function kitRuntime(root) {
	const kitRoot = kitRootFor(root);
	const transportURL = pathToFileURL(join(kitRoot, 'src/runtime/app/internal/transport.js')).href;
	const internalURL = pathToFileURL(join(kitRoot, 'src/exports/internal/shared.js')).href;
	const transport = await import(transportURL);
	const hooks = join(root, '.svelte-kit/output/server/entries/hooks.universal.js');
	let definitions = {};
	try { ({ transport: definitions = {} } = await import(pathToFileURL(hooks).href)); }
	catch (error) {
		if (error?.code !== 'ERR_MODULE_NOT_FOUND') throw error;
	}
	transport.init_transport(definitions);
	const internal = await import(internalURL);
	const require = createRequire(join(kitRoot, 'package.json'));
	const devalue = await import(pathToFileURL(require.resolve('devalue')).href);
	return { transport, internal, devalue };
}

let runtimePromise;
function runtime() {
	if (!runtimePromise) runtimePromise = kitRuntime(appRoot());
	return runtimePromise;
}

/** Called by the generated native Kit input producer. */
export async function remoteInputs(module, name) {
	try { verifyKitQueue(appRoot()); }
	catch (error) { return reportWorkerFatal(error); }
	const raw = await invokeWorker({ kind: 'remote-inputs', module, name });
	try {
		const answer = JSON.parse(raw);
		if (typeof answer.inputs !== 'string') throw makeError('skgo prerender inputs response has no devalue payload');
		const { transport } = await runtime();
		const values = transport.parse(answer.inputs);
		if (!Array.isArray(values)) throw makeError('skgo prerender inputs response did not decode to an array');
		return values;
	} catch (error) {
		return reportWorkerFatal(error);
	}
}

/** The emulator callback for a Go load, executing entirely in its Kit worker. */
export async function remoteLoad(module, source, event) {
	let kit;
	try { kit = await runtime(); } catch (error) { return reportWorkerFatal(error); }
	const { devalue, transport } = kit;
	const headers = Object.fromEntries([...event.request.headers].map(([name, value]) => [name, [value]]));
	const cookies = event.cookies.getAll();
	if (cookies.length) {
		headers.cookie = [cookies.map(({ name, value }) => `${encodeURIComponent(name)}=${encodeURIComponent(value)}`).join('; ')];
	}
	// Kit's JS matcher results cannot construct named Go values. Send its
	// actual route metadata and the unconverted path to the build-only Go load.
	const { manifest } = await import(pathToFileURL(join(appRoot(), '.svelte-kit/output/server/manifest-full.js')).href);
	const route = manifest.routes.find((candidate) => candidate.id === event.route.id);
	const prefix = manifest.app_path.slice(0, -manifest.app_dir.length);
	const base = prefix ? '/' + prefix.slice(0, -1) : '';
	const { decode_pathname } = await import(pathToFileURL(join(kitRootFor(appRoot()), 'src/utils/url.js')).href);
	const request = {
		kind: 'load', module, url: event.url.href, routeId: event.route.id,
		params: {}, parent: await event.parent(), headers,
		...(route ? { routePattern: route.pattern.source, routeParams: route.params,
			routePath: decode_pathname(event.url.pathname).slice(base.length) || '/' } : {})
	};
	const raw = await invokeWorker(request);
	const answer = JSON.parse(raw);
	if (typeof answer.failure === 'string') {
		throw new Error(`skgo: Go load failed during prerender: route ID ${event.route.id}, path ${event.url.pathname}, source ${source}: ${answer.failure}`);
	}
	for (const [name, values] of Object.entries(answer.headers ?? {})) event.setHeaders({ [name]: values.join(', ') });
	for (const cookie of answer.cookies ?? []) {
		event.cookies.set(cookie.name, cookie.value, {
			path: cookie.path, ...(cookie.domain ? { domain: cookie.domain } : {}),
			...(cookie.maxAge ? { maxAge: cookie.maxAge } : {}), httpOnly: cookie.httpOnly,
			secure: cookie.secure, sameSite: ['lax', 'lax', 'strict', 'none'][cookie.sameSite] ?? 'lax'
		});
	}
	if (answer.data) {
		const pending = new Map();
		const revivers = {
			...transport.decoders,
			Promise: (id) => {
				let resolve;
				let reject;
				const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
				pending.set(id, { promise, resolve, reject });
				return promise;
			}
		};
		try {
			answer.data = devalue.unflatten(answer.data, revivers);
			for (const chunk of answer.chunks ?? []) {
				const deferred = pending.get(chunk.id);
				if (!deferred) throw makeError(`skgo: unknown prerender deferred id ${chunk.id}`);
				if (chunk.error) deferred.reject(new Error(chunk.error));
				else deferred.resolve(devalue.unflatten(chunk.data, revivers));
			}
		} catch (error) {
			return reportWorkerFatal(error);
		}
	}
	return answer;
}

/** The emulator callback for a Go prerender remote, executing in Kit's worker. */
export async function remoteFunction(module, name, arg, event) {
	let kit;
	try { kit = await runtime(); } catch (error) { return reportWorkerFatal(error); }
	const { internal, transport } = kit;
	let payload;
	try { payload = arg === undefined ? '' : Buffer.from(transport.stringify(arg), 'utf8').toString('base64url'); }
	catch (error) { return reportWorkerFatal(error); }
	const headers = Object.fromEntries([...event.request.headers].map(([key, value]) => [key, [value]]));
	const raw = await invokeWorker({
		kind: 'remote', module, name, payload, url: event.url.href, headers
	});
	const answer = JSON.parse(raw);
	if (answer.type === 'error' && answer.kind === 'app') throw new internal.HttpError(answer.error);
	if (answer.type === 'error' && answer.kind === 'unknown') throw new Error(answer.diagnostic);
	if (answer.type === 'redirect') throw new internal.Redirect(answer.redirect.status, answer.redirect.location);
	if (answer.type !== 'result' || typeof answer.data !== 'string') throw makeError('skgo prerender remote response is malformed');
	try {
		const result = transport.parse(answer.data);
		if (!result || typeof result !== 'object' || !Object.hasOwn(result, '_')) {
			throw makeError('skgo prerender remote result omitted its value member');
		}
		return result._;
	}
	catch (error) { return reportWorkerFatal(error); }
}
