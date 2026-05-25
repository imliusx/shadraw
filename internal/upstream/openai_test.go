package upstream

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const fakePNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func TestClient_ImagesGenerations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("missing bearer; got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"b64_json":"` + fakePNG + `"}]}`))
	}))
	defer srv.Close()

	c := NewClient()
	res, err := c.Generate(context.Background(), Config{BaseURL: srv.URL, APIKey: "sk-test"}, GenerateParams{
		Model: ImagesAPIModel, Prompt: "a cat", Ratio: "1:1", Pixels: "2K",
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	want, _ := base64.StdEncoding.DecodeString(fakePNG)
	if string(res.PNG) != string(want) {
		t.Fatalf("png mismatch")
	}
}

func TestClient_ImagesEdits_WithReference(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Fatalf("expected multipart; got %s", r.Header.Get("Content-Type"))
		}
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if len(r.MultipartForm.File["image"]) == 0 {
			t.Fatal("missing image part")
		}
		w.Write([]byte(`{"data":[{"b64_json":"` + fakePNG + `"}]}`))
	}))
	defer srv.Close()

	dataURL := "data:image/png;base64," + fakePNG
	c := NewClient()
	res, err := c.Generate(context.Background(), Config{BaseURL: srv.URL, APIKey: "sk-test"}, GenerateParams{
		Model: ImagesAPIModel, Prompt: "edit", Ratio: "1:1", Pixels: "2K",
		ReferenceImages: []ReferenceImage{{DataURL: dataURL}},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(res.PNG) == 0 {
		t.Fatal("empty png")
	}
}

func TestClient_ResponsesSSE(t *testing.T) {
	sse := "event: response.image_generation_call.completed\n" +
		"data: {\"result\":\"" + fakePNG + "\"}\n\n" +
		"event: response.completed\n" +
		"data: [DONE]\n\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sse))
	}))
	defer srv.Close()

	c := NewClient()
	res, err := c.Generate(context.Background(), Config{BaseURL: srv.URL, APIKey: "sk-test"}, GenerateParams{
		Model: "gpt-5.3-codex", Prompt: "a cat", Ratio: "1:1", Pixels: "2K",
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(res.PNG) == 0 {
		t.Fatal("empty png")
	}
}

func TestClient_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient()
	_, err := c.Generate(context.Background(), Config{BaseURL: srv.URL, APIKey: "bad"}, GenerateParams{
		Model: ImagesAPIModel, Prompt: "x",
	})
	var ue *Error
	if err == nil {
		t.Fatal("expected error")
	}
	ok := false
	if e, isErr := err.(*Error); isErr {
		ue = e
		ok = true
	}
	if !ok || ue.Kind != ErrKindAuth || ue.Status != 401 {
		t.Fatalf("expected auth error, got %#v", err)
	}
}

func TestClient_TestConnection_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()
	c := NewClient()
	if err := c.TestConnection(context.Background(), Config{BaseURL: srv.URL, APIKey: "x"}); err != nil {
		t.Fatalf("test connection: %v", err)
	}
}

func TestClient_TestConnection_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := NewClient()
	err := c.TestConnection(context.Background(), Config{BaseURL: srv.URL, APIKey: "x"})
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://api.openai.com":       "https://api.openai.com",
		"https://api.openai.com/":      "https://api.openai.com",
		"https://api.openai.com/v1":    "https://api.openai.com",
		"https://api.openai.com/v1///": "https://api.openai.com",
	}
	for in, want := range cases {
		got := normalizeBaseURL(in)
		if got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
