package taiga

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func TestClientSendsBearerAndParsesPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("X-Pagination-Current", "2")
		w.Header().Set("X-Paginated-By", "10")
		w.Header().Set("X-Pagination-Count", "25")
		w.Header().Set("X-Pagination-Next", "3")
		_, _ = io.WriteString(w, `[{"id":1,"name":"Demo","slug":"demo"}]`)
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/v1/", WithToken("secret-token"))
	if err != nil {
		t.Fatal(err)
	}
	projects, page, err := client.ListProjects(context.Background(), 2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Slug != "demo" {
		t.Fatalf("projects = %#v", projects)
	}
	if page.Number != 2 || page.Size != 10 || page.Total != 25 || page.Next != 3 {
		t.Fatalf("page = %#v", page)
	}
}

func TestClientRetriesGETButNotMutation(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests < 3 {
			http.Error(w, `{"detail":"temporary"}`, http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `[]`)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL+"/", WithMaxRetries(3))
	client.sleep = func(context.Context, time.Duration) error { return nil }
	var result []any
	if _, err := client.Get(context.Background(), "projects", nil, &result); err != nil {
		t.Fatal(err)
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("connection lost")
}

func TestMutationTransportFailureIsAmbiguous(t *testing.T) {
	httpClient := &http.Client{Transport: failingTransport{}}
	client, _ := NewClient("https://example.test/api/v1/", WithHTTPClient(httpClient))
	_, err := client.Post(context.Background(), "issues", map[string]any{"subject": "x"}, nil)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Kind != KindAmbiguousCommit {
		t.Fatalf("error = %#v", err)
	}
}

func TestAPIErrorClassificationAndNoTokenInVerbose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"_error_message":"wrong version","_error_type":"WrongVersion"}`)
	}))
	defer server.Close()
	var log strings.Builder
	client, _ := NewClient(server.URL+"/", WithToken("do-not-log"), WithVerbose(&log))
	_, err := client.Patch(context.Background(), "issues/1", UpdateIssueRequest{Version: 1}, nil)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Kind != KindConflict {
		t.Fatalf("error = %#v", err)
	}
	if strings.Contains(log.String(), "do-not-log") {
		t.Fatalf("verbose log leaked token: %s", log.String())
	}
}

func TestClientRefreshesExpiredTokenAndPersistsRotation(t *testing.T) {
	meRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/users/me":
			meRequests++
			if r.Header.Get("Authorization") == "Bearer old-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"detail":"expired"}`)
				return
			}
			if r.Header.Get("Authorization") != "Bearer new-token" {
				t.Fatalf("unexpected refreshed Authorization: %q", r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, `{"id":1,"username":"demo"}`)
		case "/api/v1/auth/refresh":
			if r.Header.Get("Authorization") != "" {
				t.Fatalf("refresh leaked Authorization header")
			}
			_, _ = io.WriteString(w, `{"auth_token":"new-token","refresh":"new-refresh"}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	var persistedAuth, persistedRefresh string
	client, _ := NewClient(server.URL+"/api/v1/",
		WithToken("old-token"),
		WithRefreshToken("old-refresh", func(authToken, refreshToken string) error {
			persistedAuth, persistedRefresh = authToken, refreshToken
			return nil
		}),
	)
	user, err := client.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if user.Username != "demo" || meRequests != 2 {
		t.Fatalf("user=%#v meRequests=%d", user, meRequests)
	}
	if persistedAuth != "new-token" || persistedRefresh != "new-refresh" {
		t.Fatalf("persisted auth=%q refresh=%q", persistedAuth, persistedRefresh)
	}
}

func TestClientPropagatesCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	client, _ := NewClient(server.URL+"/", WithMaxRetries(3))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output any
	_, err := client.Get(ctx, "slow", nil, &output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %#v", err)
	}
}

func TestAPIStatusClassification(t *testing.T) {
	tests := []struct {
		status int
		kind   ErrorKind
	}{
		{status: http.StatusUnauthorized, kind: KindAuth},
		{status: http.StatusForbidden, kind: KindForbidden},
		{status: http.StatusNotFound, kind: KindNotFound},
		{status: http.StatusTooManyRequests, kind: KindThrottled},
		{status: http.StatusBadGateway, kind: KindTransport},
	}
	for _, test := range tests {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			apiErr := decodeAPIError("GET /resource", test.status, []byte(`{"detail":"failure"}`))
			if apiErr.Kind != test.kind {
				t.Fatalf("kind = %q, want %q", apiErr.Kind, test.kind)
			}
		})
	}
}

func TestTaigaVersionFieldIsConflict(t *testing.T) {
	apiErr := decodeAPIError("PATCH /issues/1", http.StatusBadRequest, []byte(`{"version":"The version doesn't match with the current one"}`))
	if apiErr.Kind != KindConflict {
		t.Fatalf("kind = %q, want %q", apiErr.Kind, KindConflict)
	}
}

func TestLoginUsesCurrentRefreshField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/auth" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var request LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Type != "normal" || request.Username != "demo" || request.Password != "password" {
			t.Fatalf("request = %#v", request)
		}
		_, _ = io.WriteString(w, `{"auth_token":"auth","refresh":"refresh","id":1,"username":"demo"}`)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL + "/api/v1/")
	response, err := client.Login(context.Background(), "demo", "password")
	if err != nil {
		t.Fatal(err)
	}
	if response.AuthToken != "auth" || response.RefreshToken != "refresh" {
		t.Fatalf("response = %#v", response)
	}
}

func TestCreateAttachmentStreamsMultipart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/issues/attachments" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("project") != "1" || r.FormValue("object_id") != "2" || r.FormValue("description") != "evidence" {
			t.Fatalf("form = %#v", r.MultipartForm.Value)
		}
		file, header, err := r.FormFile("attached_file")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = file.Close() }()
		data, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		if header.Filename != "note.txt" || string(data) != "hello" {
			t.Fatalf("filename=%q data=%q", header.Filename, data)
		}
		_, _ = io.WriteString(w, `{"id":3,"project":1,"object_id":2,"name":"note.txt","size":5,"description":"evidence"}`)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL+"/api/v1/", WithToken("token"))
	attachment, err := client.CreateAttachment(context.Background(), "issue", 1, 2, "note.txt", "evidence", false, strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if attachment.ID != 3 || attachment.Name != "note.txt" || attachment.Size != 5 {
		t.Fatalf("attachment = %#v", attachment)
	}
}

func TestWatchAndHistoryEndpoints(t *testing.T) {
	watching := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/issues/7/watch":
			watching = true
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/issues/7":
			_, _ = io.WriteString(w, fmt.Sprintf(`{"is_watcher":%t}`, watching))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/history/issue/7":
			if r.URL.Query().Get("type") != "comment" || r.URL.Query().Get("page") != "2" || r.URL.Query().Get("page_size") != "10" {
				t.Fatalf("history query = %s", r.URL.RawQuery)
			}
			w.Header().Set("X-Pagination-Current", "2")
			w.Header().Set("X-Paginated-By", "10")
			w.Header().Set("X-Pagination-Count", "11")
			_, _ = io.WriteString(w, `[{"id":"entry-1","type":1,"created_at":"2026-08-31T00:00:00Z","user":{"pk":1,"username":"demo","name":"Demo"},"comment":"note"}]`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client, _ := NewClient(server.URL + "/api/v1/")
	verified, err := client.SetWatching(context.Background(), "issue", 7, true)
	if err != nil || !verified {
		t.Fatalf("verified=%t err=%v", verified, err)
	}
	entries, page, err := client.History(context.Background(), "issue", 7, "comment", 2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != "entry-1" || entries[0].User.Username != "demo" || page.Number != 2 || page.Total != 11 {
		t.Fatalf("entries=%#v page=%#v", entries, page)
	}
}

func TestVotingEndpoints(t *testing.T) {
	voting := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/epics/7/upvote":
			voting = true
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/epics/7/downvote":
			voting = false
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/epics/7":
			_, _ = fmt.Fprintf(w, `{"is_voter":%t}`, voting)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client, _ := NewClient(server.URL + "/api/v1/")
	for _, expected := range []bool{true, false} {
		verified, err := client.SetVoting(context.Background(), "epic", 7, expected)
		if err != nil || verified != expected {
			t.Fatalf("expected=%t verified=%t err=%v", expected, verified, err)
		}
	}
}

func TestAttachmentResourcePaths(t *testing.T) {
	tests := []struct {
		resource string
		path     string
	}{
		{resource: "epic", path: "/api/v1/epics/attachments"},
		{resource: "wiki", path: "/api/v1/wiki/attachments"},
	}
	for _, test := range tests {
		t.Run(test.resource, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != test.path {
					t.Fatalf("request = %s %s", r.Method, r.URL.Path)
				}
				if r.URL.Query().Get("project") != "1" || r.URL.Query().Get("object_id") != "2" {
					t.Fatalf("query = %s", r.URL.RawQuery)
				}
				_, _ = io.WriteString(w, `[{"id":3,"project":1,"object_id":2,"name":"note.txt"}]`)
			}))
			defer server.Close()
			client, _ := NewClient(server.URL + "/api/v1/")
			attachments, err := client.ListAttachments(context.Background(), test.resource, 1, 2)
			if err != nil {
				t.Fatal(err)
			}
			if len(attachments) != 1 || attachments[0].ID != 3 {
				t.Fatalf("attachments = %#v", attachments)
			}
		})
	}
}

func TestRetryDelayHonoursRetryAfter(t *testing.T) {
	if got := retryDelay(1, "5"); got != 5*time.Second {
		t.Fatalf("retryDelay(1, \"5\") = %v, want 5s", got)
	}
	if got := retryDelay(1, "120"); got != maxRetryAfter {
		t.Fatalf("retryDelay(1, \"120\") = %v, want %v", got, maxRetryAfter)
	}
	past := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)
	if got := retryDelay(1, past); got != 0 {
		t.Fatalf("retryDelay(1, past) = %v, want 0", got)
	}
	future := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)
	if got := retryDelay(1, future); got != maxRetryAfter {
		t.Fatalf("retryDelay(1, future) = %v, want %v", got, maxRetryAfter)
	}
}

func TestRetryDelayBackoffIsBoundedAndJittered(t *testing.T) {
	const samples = 200
	for attempt := 1; attempt <= 4; attempt++ {
		base := backoffBase(attempt)
		distinct := map[time.Duration]struct{}{}
		for sample := 0; sample < samples; sample++ {
			delay := retryDelay(attempt, "")
			if delay < base || delay >= base+jitterWindow {
				t.Fatalf("retryDelay(%d, \"\") = %v, want [%v, %v)", attempt, delay, base, base+jitterWindow)
			}
			distinct[delay] = struct{}{}
		}
		if len(distinct) < 2 {
			t.Fatalf("attempt %d produced a single delay across %d samples; jitter is not random", attempt, samples)
		}
	}
}

func TestBackoffBaseDoublesWithoutOverflowing(t *testing.T) {
	if backoffBase(1) != backoffStep || backoffBase(3) != 4*backoffStep {
		t.Fatalf("backoffBase(1) = %v, backoffBase(3) = %v", backoffBase(1), backoffBase(3))
	}
	if got := backoffBase(1000); got <= 0 {
		t.Fatalf("backoffBase(1000) = %v, want a positive delay", got)
	}
}

// Every body below was captured from a real Taiga 6 server, because the whole
// classification turns on shapes that no specification promises. Taiga answers
// a stale write from its own concurrency check, which carries one sentence,
// and a malformed field through Django REST Framework, which carries a list.
// Both arrive as HTTP 400 under the same key.
func TestDecodeAPIErrorSeparatesStaleWritesFromBadOnes(t *testing.T) {
	staleWrite := `{"version": "The version doesn't match with the current one"}`
	if got := decodeAPIError("PATCH /issues/1", http.StatusBadRequest, []byte(staleWrite)); got.Kind != KindConflict {
		t.Errorf("stale write classified %s, want %s", got.Kind, KindConflict)
	}
	// Rejecting the version field is not somebody else's edit. Calling it one
	// tells the caller to re-read and retry, which never terminates because
	// re-reading does not make the value a whole number.
	badVersion := `{"version": ["Enter a whole number."]}`
	if got := decodeAPIError("PATCH /issues/1", http.StatusBadRequest, []byte(badVersion)); got.Kind != KindValidation {
		t.Errorf("malformed version classified %s, want %s", got.Kind, KindValidation)
	}
	for _, body := range []string{
		`{"subject": ["This field is required."]}`,
		`{"assigned_to": ["Invalid pk '999999' - object does not exist."]}`,
		`{"description": ["Mention the version you tested"]}`,
		`{"_error_message": "Version must be specified"}`,
		`{"_error_message": "Unsupported API version"}`,
	} {
		if got := decodeAPIError("POST /issues", http.StatusBadRequest, []byte(body)); got.Kind != KindValidation {
			t.Errorf("body %s classified %s, want %s", body, got.Kind, KindValidation)
		}
	}
}

// A conflict signal must survive translation: Taiga renders that sentence
// through Django's gettext, so a server running in another language sends
// different words under the same key.
func TestDecodeAPIErrorRecognisesAStaleWriteInAnyLanguage(t *testing.T) {
	for _, body := range []string{
		`{"version": "La version ne correspond pas à la version actuelle"}`,
		`{"version": "版本與目前的版本不符"}`,
	} {
		if got := decodeAPIError("PATCH /issues/1", http.StatusBadRequest, []byte(body)); got.Kind != KindConflict {
			t.Errorf("body %s classified %s, want %s", body, got.Kind, KindConflict)
		}
	}
}

func TestDecodeAPIErrorRendersRejectedFields(t *testing.T) {
	cases := map[string]string{
		`{"subject": ["This field is required."]}`:                      "subject: This field is required.",
		`{"assigned_to":["Invalid pk"],"subject":["Required"]}`:         "assigned_to: Invalid pk; subject: Required",
		`{"subject":["Too long.","Use fewer words."]}`:                  "subject: Too long. Use fewer words.",
		`{"non_field_errors":["Invalid data"]}`:                         "Invalid data",
		`{"version": "The version doesn't match with the current one"}`: "version: The version doesn't match with the current one",
		// Django REST Framework nests a sub-serializer's errors under the
		// field holding it, which is how Taiga reports watchers and points.
		`{"watchers":{"0":["Invalid pk 999 - object does not exist."]}}`: "watchers: 0: Invalid pk 999 - object does not exist.",
		// A many=True serializer, which is what bulk create posts, answers
		// with one entry per submitted row rather than an object.
		`[{"subject":["required"]},{"subject":["too long"]}]`: "item 0: subject: required; item 1: subject: too long",
	}
	for body, want := range cases {
		if got := decodeAPIError("POST /issues", http.StatusBadRequest, []byte(body)).Message; got != want {
			t.Errorf("body %s\n got %q\nwant %q", body, got, want)
		}
	}
}

// Field rendering belongs to validation responses. A proxy's JSON error page
// must not replace a status people recognise with its own internals.
func TestDecodeAPIErrorKeepsStatusTextForNonValidationFailures(t *testing.T) {
	for _, testCase := range []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusServiceUnavailable, `{"status":"error","code":"upstream_unavailable"}`, "Service Unavailable"},
		{http.StatusInternalServerError, `{"error":"boom","request_id":"abc123"}`, "Internal Server Error"},
		{http.StatusBadRequest, `{}`, "Bad Request"},
		{http.StatusBadRequest, `not json`, "Bad Request"},
		{http.StatusBadRequest, `{"count":3}`, "Bad Request"},
	} {
		if got := decodeAPIError("GET /issues", testCase.status, []byte(testCase.body)).Message; got != testCase.want {
			t.Errorf("status %d body %s produced %q, want %q", testCase.status, testCase.body, got, testCase.want)
		}
	}
}

// A body carrying both Taiga's prose and rejected fields must not lose the
// fields just because the prose repeats the status line.
func TestDecodeAPIErrorPrefersWhicheverSaysMore(t *testing.T) {
	if got := decodeAPIError("POST /issues", http.StatusBadRequest, []byte(`{"_error_message":"Permission denied"}`)).Message; got != "Permission denied" {
		t.Errorf("message = %q, want Taiga's own sentence", got)
	}
	body := `{"detail":"Bad Request","subject":["This field is required."]}`
	if got := decodeAPIError("POST /issues", http.StatusBadRequest, []byte(body)).Message; got != "subject: This field is required." {
		t.Errorf("message = %q, want the rejected field", got)
	}
}

// A body is read up to maxResponseBytes and a rejected bulk create carries a
// message per row, so what reaches a terminal has to be bounded.
func TestDecodeAPIErrorBoundsWhatItRenders(t *testing.T) {
	body := `{"subject":["` + strings.Repeat("x", 4*maxMessageBytes) + `"]}`
	message := decodeAPIError("POST /issues", http.StatusBadRequest, []byte(body)).Message
	if len(message) > maxMessageBytes+len("… (truncated)") {
		t.Fatalf("message is %d bytes, want it capped near %d", len(message), maxMessageBytes)
	}
	if !strings.HasSuffix(message, "(truncated)") {
		t.Errorf("a clipped message must say so, got %q", message[max(0, len(message)-40):])
	}
	if !utf8.ValidString(message) {
		t.Error("clipping must not cut a rune in half")
	}
}

func TestDecodeAPIErrorJoinsSeveralFieldsInAStableOrder(t *testing.T) {
	body := []byte(`{"subject":"required","assigned_to":"unknown user"}`)
	first := decodeAPIError("POST /issues", http.StatusBadRequest, body).Message
	if second := decodeAPIError("POST /issues", http.StatusBadRequest, body).Message; first != second {
		t.Fatalf("message is not deterministic: %q then %q", first, second)
	}
	if first != "assigned_to: unknown user; subject: required" {
		t.Fatalf("message = %q", first)
	}
}

// Interrupting a write does not un-send it. Reporting plain cancellation there
// tells a caller the write did not happen, which nobody can know.
func TestWriteInterruptedInFlightIsAmbiguous(t *testing.T) {
	client, ctx, done := interruptibleClient(t)
	defer done()

	_, err := client.Post(ctx, "issues", map[string]any{"subject": "x"}, nil)
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("interrupted write reported %v, want an *Error", err)
	}
	if apiErr.Kind != KindAmbiguousCommit {
		t.Errorf("interrupted write classified %s, want %s", apiErr.Kind, KindAmbiguousCommit)
	}
	// Every transport failure on a write reports that same kind, so asserting
	// it alone would leave this green with the interrupt handling deleted.
	if !errors.Is(err, context.Canceled) {
		t.Error("the error must carry the cancellation that produced it")
	}
	if apiErr.Message != interruptedMessage {
		t.Errorf("message = %q, want the one written for an interrupt", apiErr.Message)
	}
}

// A POST that settles nothing has nothing to reconcile, so interrupting one
// must not send a person looking for a record that never existed.
func TestInterruptedIdempotentPostIsNotAmbiguous(t *testing.T) {
	client, ctx, done := interruptibleClient(t)
	defer done()

	_, err := client.PostIdempotent(ctx, "auth", map[string]any{"username": "someone"}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	var apiErr *Error
	if errors.As(err, &apiErr) {
		t.Errorf("logging in commits nothing, so %s is wrong", apiErr.Kind)
	}
}

// A context that was already finished sent nothing, so it is just cancelled.
func TestWriteCancelledBeforeSendingIsNotAmbiguous(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should have been sent")
	}))
	defer server.Close()

	client, err := NewClient(server.URL + "/api/v1/")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = client.Post(ctx, "issues", map[string]any{"subject": "x"}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	var apiErr *Error
	if errors.As(err, &apiErr) {
		t.Errorf("nothing was sent, so %s is wrong", apiErr.Kind)
	}
}

// interruptibleClient serves a request that never answers and cancels the
// context once the server has it, which is the shape of an operator pressing
// Ctrl-C while a request is in flight.
func interruptibleClient(t *testing.T) (*Client, context.Context, func()) {
	t.Helper()
	reached := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(reached) })
		<-release
	}))
	client, err := NewClient(server.URL + "/api/v1/")
	if err != nil {
		server.Close()
		t.Fatalf("NewClient: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-reached
		cancel()
	}()
	// The handler is released before the server is closed, since Close waits
	// for it to return.
	return client, ctx, func() {
		cancel()
		close(release)
		server.Close()
	}
}

// Not every DELETE goes through Client.Delete. These four reach the request
// layer on their own, because they carry a moveTo query or share the
// delete-then-verify helper, and each destroys something. Interrupting one
// does not un-send it, so it has to warn like any other write rather than
// report that nothing happened.
func TestInterruptedDeleteOutsideTheWrapperIsAmbiguous(t *testing.T) {
	moveTo := int64(2)
	for _, testCase := range []struct {
		name   string
		remove func(*Client, context.Context) error
	}{
		{"role", func(c *Client, ctx context.Context) error {
			return c.DeleteRole(ctx, 1, &moveTo)
		}},
		{"swimlane", func(c *Client, ctx context.Context) error {
			return c.DeleteSwimlane(ctx, 1, &moveTo)
		}},
		{"due date", func(c *Client, ctx context.Context) error {
			return c.DeleteDueDate(ctx, "issue", 1)
		}},
		{"workflow metadata", func(c *Client, ctx context.Context) error {
			return c.DeleteWorkflowMetadata(ctx, "issue-status", 1, moveTo)
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			client, ctx, done := interruptibleClient(t)
			defer done()

			err := testCase.remove(client, ctx)
			var apiErr *Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("interrupted delete reported %v, want an *Error", err)
			}
			if apiErr.Kind != KindAmbiguousCommit {
				t.Errorf("interrupted delete classified %s, want %s", apiErr.Kind, KindAmbiguousCommit)
			}
			if apiErr.Message != interruptedMessage {
				t.Errorf("message = %q, want the one written for an interrupt", apiErr.Message)
			}
		})
	}
}

// Taiga retires the old refresh token the moment it issues a new one, so a
// new token this process could not store leaves the credential on disk dead.
// The 401 that started the refresh must not be what the caller hears: it says
// "log in again" without saying why a login that worked has stopped working.
func TestRefreshedTokenThatCannotBeStoredIsReported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/users/me":
			if r.Header.Get("Authorization") == "Bearer old-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"detail":"expired"}`)
				return
			}
			_, _ = io.WriteString(w, `{"id":1,"username":"demo"}`)
		case "/api/v1/auth/refresh":
			_, _ = io.WriteString(w, `{"auth_token":"new-token","refresh":"new-refresh"}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	storeErr := errors.New("keyring is locked")
	client, err := NewClient(server.URL+"/api/v1/",
		WithToken("old-token"),
		WithRefreshToken("old-refresh", func(string, string) error { return storeErr }),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.Me(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("got %v, want an *Error", err)
	}
	if apiErr.Kind != KindAuth {
		t.Errorf("kind = %s, want %s", apiErr.Kind, KindAuth)
	}
	if apiErr.Message != refreshNotStoredMessage {
		t.Errorf("message = %q, want the one that says the stored credential is stale", apiErr.Message)
	}
	if !errors.Is(err, storeErr) {
		t.Error("the error must carry why the token could not be stored")
	}
}

// A refresh that Taiga refuses leaves the caller with the rejection that
// started it, which names what they ran. A refresh that never reached Taiga,
// or came back unreadable, or was throttled, is not a refused credential and
// must not be reported as one: the refresh token is still good, and sending
// someone to log in again over a dropped connection is the wrong advice.
func TestRefreshFailuresKeepTheirOwnMeaning(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		answer    func(http.ResponseWriter)
		kind      ErrorKind
		retryable bool
		operation string
	}{
		{"connection dropped", func(w http.ResponseWriter) {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
		}, KindTransport, true, "POST /api/v1/auth/refresh"},
		{"gateway error", func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, `<html>502</html>`)
		}, KindTransport, true, "POST /api/v1/auth/refresh"},
		{"throttled", func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"detail":"slow down"}`)
		}, KindThrottled, true, "POST /api/v1/auth/refresh"},
		{"unreadable answer", func(w http.ResponseWriter) {
			_, _ = io.WriteString(w, `<html>please log in</html>`)
		}, KindTransport, false, "POST /api/v1/auth/refresh"},
		{"refused", func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"detail":"Token is invalid or expired"}`)
		}, KindAuth, false, "GET /api/v1/users/me"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/users/me":
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = io.WriteString(w, `{"detail":"Token is expired"}`)
				case "/api/v1/auth/refresh":
					testCase.answer(w)
				default:
					t.Errorf("unexpected path %q", r.URL.Path)
				}
			}))
			defer server.Close()
			client, err := NewClient(server.URL+"/api/v1/", WithToken("old"), WithRefreshToken("still-good", func(string, string) error { return nil }))
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}

			_, err = client.Me(context.Background())
			var apiErr *Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("got %v, want an *Error", err)
			}
			if apiErr.Kind != testCase.kind || apiErr.Retryable != testCase.retryable {
				t.Errorf("kind=%s retryable=%t, want %s retryable=%t", apiErr.Kind, apiErr.Retryable, testCase.kind, testCase.retryable)
			}
			if apiErr.Operation != testCase.operation {
				t.Errorf("operation = %q, want %q", apiErr.Operation, testCase.operation)
			}
			if client.refreshToken != "still-good" {
				t.Error("a refresh Taiga did not complete must leave the refresh token in place")
			}
		})
	}
}

// Two commands can find the same token expired at once. The one that waits
// for the lock finds the other's refreshed pair stored, and uses it rather than
// spend the old refresh token, which Taiga has already retired.
func TestRefreshUsesAPairAnotherProcessStoredWhileWaiting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/users/me":
			if r.Header.Get("Authorization") == "Bearer stored-by-other" {
				_, _ = io.WriteString(w, `{"id":1,"username":"demo"}`)
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"detail":"expired"}`)
		case "/api/v1/auth/refresh":
			t.Fatalf("refreshed with a refresh token another process already spent")
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	unlocked := false
	client, _ := NewClient(server.URL+"/api/v1/",
		WithToken("old-token"),
		WithRefreshToken("old-refresh", func(string, string) error {
			t.Fatalf("saved a pair when none was issued")
			return nil
		}),
		WithRefreshLock(func(context.Context) (string, string, func(), error) {
			return "stored-by-other", "other-refresh", func() { unlocked = true }, nil
		}),
	)
	user, err := client.Me(context.Background())
	if err != nil || user.Username != "demo" {
		t.Fatalf("Me() = %#v, %v", user, err)
	}
	if !unlocked {
		t.Fatal("the refresh lock was not released")
	}
}

// When the stored pair is still the refused one, the refresh happens, and the
// rotated pair is saved before the lock is released, so the next process to
// take it reads the new pair rather than the retired one.
func TestRefreshSavesTheRotatedPairWhileHoldingTheLock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/users/me":
			if r.Header.Get("Authorization") == "Bearer new-token" {
				_, _ = io.WriteString(w, `{"id":1,"username":"demo"}`)
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"detail":"expired"}`)
		case "/api/v1/auth/refresh":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["refresh"] != "stored-refresh" {
				t.Fatalf("refresh sent %q, want the stored refresh token", body["refresh"])
			}
			_, _ = io.WriteString(w, `{"auth_token":"new-token","refresh":"new-refresh"}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	var events []string
	client, _ := NewClient(server.URL+"/api/v1/",
		WithToken("old-token"),
		WithRefreshToken("old-refresh", func(authToken, refreshToken string) error {
			events = append(events, "save "+authToken+" "+refreshToken)
			return nil
		}),
		WithRefreshLock(func(context.Context) (string, string, func(), error) {
			events = append(events, "lock")
			return "old-token", "stored-refresh", func() { events = append(events, "unlock") }, nil
		}),
	)
	if _, err := client.Me(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := []string{"lock", "save new-token new-refresh", "unlock"}; strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestRefreshReportsALockThatCannotBeTaken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/refresh" {
			t.Fatalf("refreshed without the lock")
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"detail":"expired"}`)
	}))
	defer server.Close()
	lockErr := errors.New("lock unavailable")
	client, _ := NewClient(server.URL+"/api/v1/",
		WithToken("old-token"),
		WithRefreshToken("old-refresh", func(string, string) error { return nil }),
		WithRefreshLock(func(context.Context) (string, string, func(), error) { return "", "", nil, lockErr }),
	)
	if _, err := client.Me(context.Background()); !errors.Is(err, lockErr) {
		t.Fatalf("Me() = %v, want the lock error", err)
	}
}
