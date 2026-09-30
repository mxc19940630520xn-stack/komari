package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMonitoringRoutesKeepThemesAndProbes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r)
	routes := map[string]bool{}
	for _, route := range r.Routes() {
		routes[route.Method+" "+route.Path] = true
		for _, removed := range []string{"terminal", "/file/", "/transfer/", "/plugin", "/clipboard", "/notification", "/task/exec", "/pprof", "xtermjs", "message-sender"} {
			if strings.Contains(route.Path, removed) {
				t.Errorf("removed route still registered: %s %s", route.Method, route.Path)
			}
		}
	}
	for _, required := range []string{
		"GET /api/nodes", "GET /api/clients", "GET /api/records/ping", "GET /api/records/load",
		"GET /api/clients/v2/rpc", "POST /api/clients/v2/rpc", "POST /api/clients/register",
		"POST /api/admin/client/:uuid/edit", "POST /api/admin/ping/add",
		"GET /api/admin/theme/list", "GET /api/admin/theme/set", "POST /api/admin/theme/import",
		"POST /api/admin/theme/settings", "POST /api/admin/theme/market/install",
		"POST /api/admin/upload/init", "POST /api/admin/upload/chunk", "POST /api/admin/upload/merge",
		"POST /api/login", "GET /api/admin/2fa/generate", "GET /api/admin/download/backup",
	} {
		if !routes[required] {
			t.Errorf("required route missing: %s", required)
		}
	}
	for _, removed := range []struct{ method, path string }{
		{http.MethodGet, "/api/clients/terminal"},
		{http.MethodPost, "/api/admin/task/exec"},
		{http.MethodGet, "/api/admin/client/test/file/download"},
		{http.MethodGet, "/api/admin/plugin/list"},
	} {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(removed.method, removed.path, nil))
		if response.Code != http.StatusNotFound || !strings.Contains(response.Header().Get("Content-Type"), "application/json") {
			t.Errorf("removed endpoint %s returned %d %s", removed.path, response.Code, response.Body.String())
		}
	}
}

// Delay the body until after headers, like clients that send these separately.
// A rejected POST must still receive the JSON error, not a connection reset.
type delayedRequestBody struct{ io.Reader }

func (r delayedRequestBody) Read(p []byte) (int, error) {
	time.Sleep(5 * time.Millisecond)
	return r.Reader.Read(p)
}

func TestRemovedPostReturnsJSONWithConnectionClose(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r)
	server := httptest.NewServer(r)
	defer server.Close()
	client := server.Client()
	client.Timeout = 3 * time.Second
	for attempt := 0; attempt < 10; attempt++ {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/admin/task/exec", delayedRequestBody{strings.NewReader(`{"command":"ignored"}`)})
		if err != nil {
			t.Fatal(err)
		}
		req.ContentLength = int64(len(`{"command":"ignored"}`))
		req.Close = true
		response, err := client.Do(req)
		if err != nil {
			t.Fatalf("rejected POST lost its response: %v", err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusNotFound || !strings.Contains(string(body), "API endpoint not found") {
			t.Fatalf("unexpected rejection: status=%d, body=%s, error=%v", response.StatusCode, body, err)
		}
	}
}
