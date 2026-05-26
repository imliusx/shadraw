package upstream

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/liusx/shadraw/internal/imagegen"
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
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["size"] != "1024x1024" || body["quality"] != "medium" || body["n"] != float64(1) {
			t.Fatalf("unexpected body %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"b64_json":"` + fakePNG + `"}]}`))
	}))
	defer srv.Close()

	c := NewClient()
	res, err := c.Generate(context.Background(), Config{BaseURL: srv.URL, APIKey: "sk-test"}, GenerateParams{
		Model:  ImagesAPIModel,
		Prompt: "a cat",
		ImageParams: imagegen.Params{
			Size:    "1024x1024",
			Quality: imagegen.QualityMedium,
		},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	want, _ := base64.StdEncoding.DecodeString(fakePNG)
	if string(res.Image) != string(want) {
		t.Fatalf("png mismatch")
	}
}

func TestClient_ImagesGenerations_OfficialParams(t *testing.T) {
	compression := 80
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		want := map[string]any{
			"model":              ImagesAPIModel,
			"prompt":             "a cat",
			"size":               "1536x1024",
			"quality":            "high",
			"n":                  float64(1),
			"background":         "transparent",
			"moderation":         "low",
			"output_format":      "webp",
			"output_compression": float64(80),
		}
		for key, expected := range want {
			if body[key] != expected {
				t.Fatalf("%s = %#v, want %#v; body %#v", key, body[key], expected, body)
			}
		}
		w.Write([]byte(`{"data":[{"b64_json":"` + fakePNG + `"}]}`))
	}))
	defer srv.Close()

	c := NewClient()
	res, err := c.Generate(context.Background(), Config{BaseURL: srv.URL, APIKey: "sk-test"}, GenerateParams{
		Model:  ImagesAPIModel,
		Prompt: "a cat",
		ImageParams: imagegen.Params{
			Size:              "1536x1024",
			Quality:           imagegen.QualityHigh,
			N:                 3,
			Background:        imagegen.BackgroundTransparent,
			Moderation:        imagegen.ModerationLow,
			OutputFormat:      imagegen.OutputFormatWebP,
			OutputCompression: &compression,
		},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if res.MIME != "image/webp" || res.Extension != "webp" {
		t.Fatalf("unexpected result format: %#v", res)
	}
	if len(res.Image) == 0 {
		t.Fatal("empty png")
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
		if got := r.MultipartForm.Value["size"]; len(got) != 1 || got[0] != "1024x1024" {
			t.Fatalf("size field = %#v", got)
		}
		w.Write([]byte(`{"data":[{"b64_json":"` + fakePNG + `"}]}`))
	}))
	defer srv.Close()

	dataURL := "data:image/png;base64," + fakePNG
	c := NewClient()
	res, err := c.Generate(context.Background(), Config{BaseURL: srv.URL, APIKey: "sk-test"}, GenerateParams{
		Model:  ImagesAPIModel,
		Prompt: "edit",
		ImageParams: imagegen.Params{
			Size:    "1024x1024",
			Quality: imagegen.QualityMedium,
		},
		ReferenceImages: []ReferenceImage{{DataURL: dataURL}},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(res.Image) == 0 {
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
		var body struct {
			Tools []map[string]any `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(body.Tools) != 1 || body.Tools[0]["size"] != "1024x1024" || body.Tools[0]["quality"] != "medium" {
			t.Fatalf("unexpected tools %#v", body.Tools)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sse))
	}))
	defer srv.Close()

	c := NewClient()
	res, err := c.Generate(context.Background(), Config{BaseURL: srv.URL, APIKey: "sk-test"}, GenerateParams{
		Model:  "gpt-5.3-codex",
		Prompt: "a cat",
		ImageParams: imagegen.Params{
			Size:    "1024x1024",
			Quality: imagegen.QualityMedium,
		},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(res.Image) == 0 {
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
