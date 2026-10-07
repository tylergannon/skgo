package skgo

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/signal"
	"time"
)

// RunPrerenderService serves the generated application's build operations until
// its build owner closes the dedicated inherited control channel. It does not
// depend on the application's final frontend manifest.
func RunPrerenderService(transport Transport, loads []*ServerLoad, remotes []*Remote, options PrerenderServiceOptions) error {
	if options.BindRequest == nil {
		return fmt.Errorf("skgo: prerender service requires BindRequest")
	}
	control, err := prerenderControl()
	if err != nil {
		return fmt.Errorf("skgo: open prerender control channel: %w", err)
	}
	defer control.Close()
	ctx, stop := signal.NotifyContext(context.Background(), prerenderSignals()...)
	defer stop()
	return servePrerenderService(ctx, control, transport, loads, remotes, options)
}

func servePrerenderService(ctx context.Context, control io.ReadWriteCloser, transport Transport, loads []*ServerLoad, remotes []*Remote, options PrerenderServiceOptions) error {
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return fmt.Errorf("skgo: generate prerender secret: %w", err)
	}
	secret := hex.EncodeToString(secretBytes)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("skgo: listen for prerender work: %w", err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	failed := make(chan error, 1)
	requests := &prerenderRequests{ctx: ctx, options: options, entries: map[string]*prerenderRequest{}}
	defer requests.close()
	server := &http.Server{
		Handler: prerenderHandler(secret, transport, loads, remotes, func(err error) {
			if ctx.Err() == nil {
				select {
				case failed <- err:
				default:
				}
			}
		}, requests),
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 5 * time.Second,
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	if err := json.NewEncoder(control).Encode(struct {
		URL    string `json:"url"`
		Secret string `json:"secret"`
	}{"http://" + listener.Addr().String(), secret}); err != nil {
		server.Close()
		return fmt.Errorf("skgo: announce prerender readiness: %w", err)
	}
	ownerGone := make(chan struct{})
	go func() { io.Copy(io.Discard, control); close(ownerGone) }()
	var failure error
	select {
	case <-ownerGone:
	case <-ctx.Done():
	case failure = <-failed:
	case err := <-served:
		if err != http.ErrServerClosed {
			failure = fmt.Errorf("skgo: prerender server stopped: %w", err)
		}
	}
	if failure == nil {
		select {
		case failure = <-failed:
		default:
		}
	}
	cancel()
	shutdownCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	if err := server.Shutdown(shutdownCtx); err != nil {
		server.Close()
		return errors.Join(failure, fmt.Errorf("skgo: prerender shutdown: %w", err))
	}
	return failure
}

func prerenderHandler(secret string, transport Transport, loads []*ServerLoad, remotes []*Remote, abandoned func(error), sessions ...*prerenderRequests) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "Bearer " + secret
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(expected)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		lifecycle := r.URL.Path == "/begin" || r.URL.Path == "/response" || r.URL.Path == "/end"
		if r.URL.Path != "/load" && r.URL.Path != "/remote" && r.URL.Path != "/inputs" && r.URL.Path != "/endpoint" && r.URL.Path != "/cookies" && !lifecycle {
			http.NotFound(w, r)
			return
		}
		callbackContext := r.Context()
		if abandoned != nil {
			var identity struct {
				Module string `json:"module"`
				Name   string `json:"name"`
			}
			json.Unmarshal(raw, &identity)
			report := func() {
				abandoned(fmt.Errorf("skgo: prerender %s %s#%s: callback canceled before response completion: %w", r.URL.Path, identity.Module, identity.Name, callbackContext.Err()))
			}
			finished := make(chan struct{})
			defer close(finished)
			defer func() {
				if callbackContext.Err() != nil {
					report()
				}
			}()
			go func() {
				select {
				case <-finished:
				case <-callbackContext.Done():
					select {
					case <-finished:
						return
					default:
					}
					report()
				}
			}()
		}
		var answer bytes.Buffer
		ctx := r.Context()
		var boundRequest *prerenderRequest
		if len(sessions) > 0 && !lifecycle && r.URL.Path != "/inputs" {
			var identity struct {
				Handle string `json:"handle"`
			}
			json.Unmarshal(raw, &identity)
			entry, acquireErr := sessions[0].acquire(identity.Handle)
			if acquireErr != nil {
				http.Error(w, acquireErr.Error(), 500)
				return
			}
			boundRequest = entry
			defer entry.active.Done()
			ctx, cancel := context.WithCancel(entry.request.Context())
			stop := context.AfterFunc(r.Context(), cancel)
			defer func() { stop(); cancel() }()
			ctx = context.WithValue(ctx, prerenderRequestKey{}, entry.request)
			// The request context carries the app locals and cancellation.
			r = r.WithContext(ctx)
		}
		ctx = r.Context()
		switch {
		case lifecycle && len(sessions) > 0:
			err = sessions[0].operation(ctx, r.URL.Path, raw, &answer)
		case r.URL.Path == "/load":
			err = runPrerenderLoad(r.Context(), bytes.NewReader(raw), &answer, transport, loads)
		case r.URL.Path == "/remote":
			err = runPrerenderRemote(r.Context(), raw, &answer, transport, remotes)
		case r.URL.Path == "/endpoint":
			if boundRequest == nil {
				err = fmt.Errorf("skgo: endpoint needs a logical request")
			} else {
				err = runPrerenderEndpoint(r.Context(), boundRequest, &answer)
			}
		case r.URL.Path == "/cookies":
			err = runPrerenderCookies(r.Context(), raw, &answer)
		case r.URL.Path == "/inputs":
			err = runPrerenderInputs(r.Context(), raw, &answer, transport, remotes)
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(struct {
				Error string `json:"error"`
			}{err.Error()})
			return
		}
		w.Write(answer.Bytes())
	})
}
