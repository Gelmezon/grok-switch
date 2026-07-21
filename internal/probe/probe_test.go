package probe

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTestModelSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]string{"content": "ok"}}},
		})
	}))
	defer srv.Close()

	r := TestModel(srv.URL+"/v1", "sk-test", "grok-4", 5*time.Second)
	if !r.OK {
		t.Fatalf("expected OK: %+v", r)
	}
}

func TestTestModelAuthFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()

	r := TestModel(srv.URL+"/v1", "bad", "grok-4", 5*time.Second)
	if r.OK {
		t.Fatal("expected failure")
	}
}
