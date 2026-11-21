package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

type unavailableStore struct{ Store }

func (s unavailableStore) Ready(context.Context) error { return errors.New("unavailable") }
func TestReadinessAndMetrics(t *testing.T) {
	h := api(newMemory())
	post := httptest.NewRecorder()
	h.ServeHTTP(post, httptest.NewRequest("POST", "/api/rides", bytes.NewBufferString(`{"pickup":{"lat":12.9716,"lng":77.5946},"destination":{"lat":12.95,"lng":77.61}}`)))
	if post.Code != 201 || post.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing request correlation")
	}
	ops := httptest.NewRecorder()
	h.ServeHTTP(ops, httptest.NewRequest("GET", "/api/ops", nil))
	var result operations
	if e := json.Unmarshal(ops.Body.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	if result.Assigned != 1 || result.SampleCount != 1 || result.Requests != 1 {
		t.Fatalf("bad metrics: %+v", result)
	}
	metrics := httptest.NewRecorder()
	h.ServeHTTP(metrics, httptest.NewRequest("GET", "/api/metrics", nil))
	for _, v := range []string{`vyro_dispatch_total{outcome="assigned"} 1`, `vyro_dispatch_duration_seconds_count 1`, `route="POST /api/rides"`} {
		if !strings.Contains(metrics.Body.String(), v) {
			t.Fatalf("metric missing: %s", v)
		}
	}
	unhealthy := api(unavailableStore{newMemory()})
	w := httptest.NewRecorder()
	unhealthy.ServeHTTP(w, httptest.NewRequest("GET", "/api/health", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	unhealthy.ServeHTTP(w, httptest.NewRequest("GET", "/api/live", nil))
	if w.Code != 200 {
		t.Fatal("liveness must not depend on storage")
	}
}
