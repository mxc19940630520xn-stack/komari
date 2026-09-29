package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
