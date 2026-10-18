package gemini

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFactCleansText(t *testing.T) {
	var gotModel bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Errorf("missing key")
		}
		body, _ := io.ReadAll(r.Body)
		raw := string(body)
		gotModel = strings.Contains(raw, `"model":"gemini-3.5-flash-lite"`) && !strings.Contains(raw, "google_search")
		if !strings.HasSuffix(r.URL.Path, "/interactions") && r.URL.Path != "/" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"ignored","steps":[{"type":"model_output","content":[{"type":"text","text":"Веник замачивают в тёплой воде.\n\nИсточник: example.com"}]}]}`))
	}))
	defer srv.Close()

	c := &Client{APIKey: "test-key", Model: "gemini-3.5-flash-lite", URL: srv.URL, HTTP: srv.Client()}
	text, err := c.Fact(context.Background(), []string{"старый факт"})
	if err != nil {
		t.Fatal(err)
	}
	if !gotModel {
		t.Fatal("request did not use the model without search")
	}
	if text != "Веник замачивают в тёплой воде." {
		t.Fatalf("got %q", text)
	}
}
