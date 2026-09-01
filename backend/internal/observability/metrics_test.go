package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsExposeStableLabelsAndCounters(t *testing.T) {
	metrics := New()
	metrics.ObserveHTTP("GET /health", http.MethodGet, "200", 10*time.Millisecond)
	metrics.LoginFailed()
	metrics.ShoppingItemCreated()

	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d", response.Code)
	}
	for _, expected := range []string{
		`homeflow_http_requests_total{method="GET",route="GET /health",status="200"} 1`,
		`homeflow_login_failures_total 1`,
		`homeflow_shopping_items_created_total 1`,
	} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("missing metric %q", expected)
		}
	}
}
