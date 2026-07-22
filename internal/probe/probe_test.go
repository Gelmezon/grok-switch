package probe

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Gelmezon/grok-switch/internal/profiles"
)

func TestTestModelSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"choices": []map[string]interface{}{{"message": map[string]string{"content": "ok"}}},
			})
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]string{{"id": "grok-4"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	r := TestModel(srv.URL+"/v1", "sk-test", "grok-4", 5*time.Second)
	if !r.OK || !r.AuthenticationOK || !r.ModelsEndpointOK || !r.ChatCompletionOK {
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
	if r.AuthenticationOK || r.ModelsEndpointOK || r.ChatCompletionOK {
		t.Fatalf("all checks should fail: %+v", r)
	}
}

func TestModelsSuccessDoesNotHideMissingChatCompletions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			http.NotFound(w, r)
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]string{{"id": "grok-4"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	r := TestModel(srv.URL+"/v1", "sk-test", "grok-4", 5*time.Second)
	if r.OK || r.ChatCompletionOK {
		t.Fatalf("chat failure must fail the overall result: %+v", r)
	}
	if !r.AuthenticationOK || !r.ModelsEndpointOK {
		t.Fatalf("models reachability should be reported separately: %+v", r)
	}
}

func TestChatTwoHundredWithoutCompletionPayloadFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
	}))
	defer srv.Close()

	r := TestModel(srv.URL+"/v1", "sk-test", "grok-4", 5*time.Second)
	if r.OK || r.ChatCompletionOK {
		t.Fatalf("invalid chat payload must not pass: %+v", r)
	}
	if !r.AuthenticationOK || !r.ModelsEndpointOK {
		t.Fatalf("successful HTTP endpoints should still report reachability: %+v", r)
	}
}

func TestUnsupportedProtocolFailsExplicitly(t *testing.T) {
	r := TestProfile(profiles.Profile{
		UpstreamFormat: "anthropic_messages",
		BaseURL:        "https://example.com",
		APIKey:         "sk-test",
		DefaultModel:   "grok-4",
	}, time.Second)
	if r.OK || !strings.Contains(r.Message, "当前仅支持 OpenAI Chat Completions") {
		t.Fatalf("unexpected result: %+v", r)
	}
}

func TestFetchModelsStandardResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("authorization header missing")
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"z-model"},{"id":"grok-4.5-latest"},{"id":"z-model"}]}`))
	}))
	defer srv.Close()

	ids, err := FetchModels(srv.URL, "sk-test", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"grok-4.5-latest", "z-model"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("models = %v", ids)
	}
}

func TestFetchModelsRelayVariants(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"models envelope", `{"models":[{"model":"model-b"},{"name":"model-a"}]}`, "model-a,model-b"},
		{"top-level array", `["model-b",{"id":"model-a"}]`, "model-a,model-b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			ids, err := FetchModels(srv.URL+"/v1", "sk-test", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(ids, ",") != tt.want {
				t.Fatalf("models = %v", ids)
			}
		})
	}
}

func TestFetchModelsRejectsAuthenticationFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()
	if _, err := FetchModels(srv.URL, "bad-key", time.Second); err == nil || !strings.Contains(err.Error(), "认证失败") {
		t.Fatalf("error = %v", err)
	}
}
