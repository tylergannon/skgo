package actions

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"sync"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

const workspaceCookie = "skgo_actions_workspace"
const signInCookie = "skgo_actions_signin"
const privateFailureMarker = "private-actions-database-token-4731"

type Profile struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Biography string `json:"biography"`
	State     string `json:"state"`
}

type PageData struct {
	Profile      Profile `json:"profile"`
	ActionCookie string  `json:"actionCookie"`
}

type SaveResult struct {
	Receipt string              `json:"receipt"`
	Price   businesslogic.Money `json:"price"`
}

type UploadResult struct {
	Filename  string              `json:"filename"`
	Bytes     int64               `json:"bytes"`
	SHA256    string              `json:"sha256"`
	Interests []string            `json:"interests"`
	Submitter string              `json:"submitter"`
	Price     businesslogic.Money `json:"price"`
}

type InspectResult struct {
	Interests []string            `json:"interests"`
	Submitter string              `json:"submitter"`
	Encoding  string              `json:"encoding"`
	Price     businesslogic.Money `json:"price"`
}

func upload(ctx context.Context) (UploadResult, error) {
	r := skgo.EventFrom(ctx).Request()
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return UploadResult{}, skgo.Errorf(http.StatusBadRequest, "Invalid multipart form")
	}
	file, header, err := r.FormFile("upload")
	if err != nil {
		return UploadResult{}, skgo.Errorf(http.StatusBadRequest, "Choose a file")
	}
	defer file.Close()
	hash := sha256.New()
	count, err := io.Copy(hash, file)
	if err != nil {
		return UploadResult{}, err
	}
	return UploadResult{
		Filename: header.Filename, Bytes: count,
		SHA256:    hex.EncodeToString(hash.Sum(nil))[:12],
		Interests: r.MultipartForm.Value["interest"],
		Submitter: r.PostForm.Get("uploadButton"),
		Price:     businesslogic.Money{Cents: 750},
	}, nil
}

func inspect(ctx context.Context) (InspectResult, error) {
	r := skgo.EventFrom(ctx).Request()
	encoding := "application/x-www-form-urlencoded"
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		encoding = "multipart/form-data"
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			return InspectResult{}, skgo.Errorf(http.StatusBadRequest, "Invalid multipart form")
		}
	} else if err := r.ParseForm(); err != nil {
		return InspectResult{}, skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	return InspectResult{
		Interests: r.PostForm["interest"], Submitter: r.PostForm.Get("encodingButton"),
		Encoding: encoding, Price: businesslogic.Money{Cents: 750},
	}, nil
}

type ValidationFailure struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Biography  string `json:"biography"`
	EmailError string `json:"emailError"`
}

var profiles = struct {
	sync.Mutex
	byWorkspace map[string]Profile
}{byWorkspace: map[string]Profile{}}

func workspace(ctx context.Context) (string, error) {
	event := skgo.EventFrom(ctx)
	if id, ok := event.Cookie(workspaceCookie); ok && id != "" {
		return id, nil
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(raw)
	if err := event.SetCookie(workspaceCookie, id, skgo.CookieOptions{Path: "/"}); err != nil {
		return "", err
	}
	return id, nil
}

func fixture(id string) Profile {
	profiles.Lock()
	defer profiles.Unlock()
	if profile, ok := profiles.byWorkspace[id]; ok {
		return profile
	}
	profile := Profile{Name: "Ada Lovelace", Email: "ada@example.test", Biography: "First programmer", State: "Active"}
	profiles.byWorkspace[id] = profile
	return profile
}

func pageLoad(ctx context.Context) (PageData, error) {
	id, err := workspace(ctx)
	if err != nil {
		return PageData{}, err
	}
	cookie, _ := skgo.EventFrom(ctx).Cookie("skgo_actions_feedback")
	return PageData{Profile: fixture(id), ActionCookie: cookie}, nil
}

func save(ctx context.Context) (SaveResult, error) {
	id, err := workspace(ctx)
	if err != nil {
		return SaveResult{}, err
	}
	request := skgo.EventFrom(ctx).Request()
	if err := request.ParseForm(); err != nil {
		return SaveResult{}, skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	profile := Profile{
		Name:      request.PostForm.Get("name"),
		Email:     request.PostForm.Get("email"),
		Biography: request.PostForm.Get("biography"),
		State:     "Active",
	}
	address, parseErr := mail.ParseAddress(profile.Email)
	if parseErr != nil || address.Address != profile.Email || !strings.Contains(strings.SplitAfter(profile.Email, "@")[1], ".") {
		return SaveResult{}, skgo.Fail(http.StatusUnprocessableEntity, ValidationFailure{
			Name: profile.Name, Email: profile.Email, Biography: profile.Biography,
			EmailError: "Enter a valid email address",
		})
	}
	profiles.Lock()
	profiles.byWorkspace[id] = profile
	profiles.Unlock()
	if err := skgo.EventFrom(ctx).SetHeader("x-skgo-action-demo", "profile-saved"); err != nil {
		return SaveResult{}, err
	}
	return SaveResult{Receipt: "Saved " + profile.Name, Price: businesslogic.Money{Cents: 750}}, nil
}

type CookieResult struct {
	Receipt string `json:"receipt"`
}

func remember(ctx context.Context) (CookieResult, error) {
	if err := skgo.EventFrom(ctx).SetCookie("skgo_actions_feedback", "violet-42", skgo.CookieOptions{Path: "/actions"}); err != nil {
		return CookieResult{}, err
	}
	return CookieResult{Receipt: "Remembered violet-42"}, nil
}

// The name remote is valid for a classic action. A native /remote=<id>
// selector reaches the remote form first; a JSON action request reaches this.
func remote(context.Context) (CookieResult, error) {
	return CookieResult{Receipt: "Classic remote-named action answered by Go"}, nil
}

func archive(ctx context.Context) error {
	id, err := workspace(ctx)
	if err != nil {
		return err
	}
	profile := fixture(id)
	profile.State = "Archived"
	profiles.Lock()
	profiles.byWorkspace[id] = profile
	profiles.Unlock()
	return nil
}

func signIn(ctx context.Context) error {
	request := skgo.EventFrom(ctx).Request()
	if err := request.ParseForm(); err != nil {
		return skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	if request.PostForm.Get("username") != "ada" {
		return skgo.Errorf(http.StatusBadRequest, "Unknown demo user")
	}
	if err := skgo.EventFrom(ctx).SetCookie(signInCookie, "ada", skgo.CookieOptions{Path: "/actions"}); err != nil {
		return err
	}
	return &skgo.Redirect{Status: http.StatusSeeOther, Location: "/actions/signed-in"}
}

func forbidden(context.Context) error {
	return skgo.Errorf(http.StatusForbidden, "You cannot edit this profile")
}

func unavailable(context.Context) error {
	return errors.New(privateFailureMarker)
}

var _ = skgo.Load(pageLoad)
var _ = skgo.ActionWithFailure(save, ValidationFailure{})
var _ = skgo.Action(upload)
var _ = skgo.Action(inspect)
var _ = skgo.Action(remember)
var _ = skgo.Action(remote)
var _ = skgo.ActionNoData(archive)
var _ = skgo.ActionNoData(signIn)
var _ = skgo.ActionNoData(forbidden)
var _ = skgo.ActionNoData(unavailable)

// The option examples share the same Go fixture and mutations. Their page
// options, rather than a second action implementation, determine the result.
func OptionLoad(ctx context.Context) (PageData, error)   { return pageLoad(ctx) }
func OptionSave(ctx context.Context) (SaveResult, error) { return save(ctx) }
func OptionSignIn(ctx context.Context) error             { return signIn(ctx) }
func OptionForbidden(ctx context.Context) error          { return forbidden(ctx) }
func OptionUnavailable(ctx context.Context) error        { return unavailable(ctx) }
