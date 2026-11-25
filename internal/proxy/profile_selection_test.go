package proxy

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"bitbucket.org/atlassian-developers/proximity/internal/config"
	"github.com/go-chi/chi"
)

func TestProfileSelectionSnapshotsEachRequest(t *testing.T) {
	var selected atomic.Value
	selected.Store("default")
	s := &server{Options: Options{Vars: map[string]any{"defaultProfile": "original"}, DefaultProfile: func() string { return selected.Load().(string) }}}
	request := httptest.NewRequest("POST", "/test", nil)
	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, chi.NewRouteContext()))
	first, err := s.buildTemplateInputFromRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	selected.Store("shared-eval")
	second, err := s.buildTemplateInputFromRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if got := first["globalVars"].(map[string]any)["defaultProfile"]; got != "default" {
		t.Fatalf("first request profile = %v", got)
	}
	if got := second["globalVars"].(map[string]any)["defaultProfile"]; got != "shared-eval" {
		t.Fatalf("second request profile = %v", got)
	}
	if got := s.Vars["defaultProfile"]; got != "original" {
		t.Fatalf("original vars were mutated: %v", got)
	}
}

func TestProfileSwitchRoutesRequestsWithoutRestart(t *testing.T) {
	requests := make(chan http.Header, 8)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	cfg, err := config.Load(filepath.Join("..", "..", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.BaseEndpoint = strconv.Quote(upstream.URL)
	for i, header := range cfg.Overrides.Global.Request.Headers {
		if header.Name == "Authorization" {
			cfg.Overrides.Global.Request.Headers[i].Input = config.Input{Text: "slauth test"}
		}
	}
	var selected atomic.Value
	var cycle atomic.Uint64
	selected.Store("default")
	s := New(Options{Logger: log.New(io.Discard, "", 0), Config: cfg, DefaultProfile: func() string {
		if selected.Load().(string) == "auto" {
			return []string{"default", "shared-eval"}[(cycle.Add(1)-1)%2]
		}
		return selected.Load().(string)
	}, Vars: map[string]any{
		"profiles": []map[string]any{
			{"name": "default", "useCaseId": "own", "atlassianCloudId": "own-cloud"},
			{"name": "shared-eval", "useCaseId": evaluationUseCaseForTest, "atlassianCloudId": "eval-cloud"},
		},
	}}).(*server)
	routes, err := s.combineCommonUriConfigs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for path, methods := range routes {
		for method, route := range methods {
			s.router.Method(method, path, s.handleEndpoint(route))
		}
	}
	local := httptest.NewServer(s.router)
	defer local.Close()
	post := func(path, wantCase, wantCloud string) {
		t.Helper()
		resp, err := http.Post(local.URL+path, "application/json", strings.NewReader(`{"model":"gpt-6.2-sol","input":"test"}`))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d", resp.StatusCode)
		}
		headers := <-requests
		if got := headers.Get("X-Atlassian-UseCaseId"); got != wantCase {
			t.Fatalf("use case = %q, want %q", got, wantCase)
		}
		if got := headers.Get("X-Atlassian-CloudId"); got != wantCloud {
			t.Fatalf("cloud ID = %q, want %q", got, wantCloud)
		}
	}
	post("/openai/v1/responses", "own", "own-cloud")
	selected.Store("shared-eval")
	post("/openai/v1/responses", evaluationUseCaseForTest, "eval-cloud")
	post("/p/default/openai/v1/responses", "own", "own-cloud")
	selected.Store("auto")
	post("/p/default/openai/v1/responses", "own", "own-cloud")
	post("/openai/v1/responses", "own", "own-cloud")
	post("/openai/v1/responses", evaluationUseCaseForTest, "eval-cloud")
	post("/p/default/openai/v1/responses", "own", "own-cloud")
	post("/openai/v1/responses", "own", "own-cloud")
}

const evaluationUseCaseForTest = "ai-gateway-eval-use-case"
