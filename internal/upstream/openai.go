// Package upstream is the server-side proxy to the OpenAI-compatible generation
// API. Requests originate from the worker pool with admin-configured credentials
// (never user credentials).
package upstream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"
)

const (
	defaultTimeout       = 5 * time.Minute
	sseEvtImageGen       = "response.image_generation_call"
	systemPromptForCodex = "你是一个图片生成助手。用户要求你生成图片时,你必须调用 image_generation 工具来生成图片,不要用文字描述图片内容。直接生成图片,不要多说任何话。"
)

// Image generation model that takes the OpenAI /v1/images path.
const ImagesAPIModel = "gpt-image-2"

// ErrorKind classifies upstream errors for retry / surfacing decisions.
type ErrorKind string

const (
	ErrKindAuth        ErrorKind = "auth_failed"
	ErrKindRateLimited ErrorKind = "rate_limited"
	ErrKindBadRequest  ErrorKind = "bad_request"
	ErrKindNotFound    ErrorKind = "not_found"
	ErrKindNetwork     ErrorKind = "network"
	ErrKindUnknown     ErrorKind = "unknown"
)

// Error wraps a classified upstream error.
type Error struct {
	Kind    ErrorKind
	Status  int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("upstream %s (%d): %s", e.Kind, e.Status, e.Message)
}

// Config holds the admin-managed upstream credentials and base URL.
type Config struct {
	BaseURL string
	APIKey  string
}

// GenerateParams is the per-request input.
type GenerateParams struct {
	Model           string
	Prompt          string
	Ratio           string
	Pixels          string
	ReferenceImages []ReferenceImage
}

// ReferenceImage is a data-url backed reference (matches the front-end format).
type ReferenceImage struct {
	DataURL string // "data:image/png;base64,..."
}

// GenerateResult is what comes back from a successful upstream call.
type GenerateResult struct {
	PNG       []byte
	ElapsedMs int64
}

// Client is the upstream HTTP client. Use NewClient to construct.
type Client struct {
	http *http.Client
}

func NewClient() *Client {
	return &Client{
		http: &http.Client{Timeout: defaultTimeout},
	}
}

// Generate dispatches to the right upstream path based on the model.
//   - gpt-image-2  → POST /v1/images/{generations,edits}
//   - everything else → POST /v1/responses with SSE
func (c *Client) Generate(ctx context.Context, cfg Config, p GenerateParams) (*GenerateResult, error) {
	start := time.Now()
	if cfg.BaseURL == "" || cfg.APIKey == "" {
		return nil, &Error{Kind: ErrKindBadRequest, Status: 0, Message: "upstream config not set"}
	}
	base := normalizeBaseURL(cfg.BaseURL)

	var png []byte
	var err error
	if p.Model == ImagesAPIModel {
		png, err = c.callImages(ctx, base, cfg.APIKey, p)
	} else {
		png, err = c.callResponses(ctx, base, cfg.APIKey, p)
	}
	if err != nil {
		return nil, err
	}
	return &GenerateResult{PNG: png, ElapsedMs: time.Since(start).Milliseconds()}, nil
}

// TestConnection probes GET <base>/v1/models. Returns nil on 2xx/4xx (4xx is
// usually "your key is wrong" which is still a "connection works" signal),
// classified Error on transport failure / 5xx.
func (c *Client) TestConnection(ctx context.Context, cfg Config) error {
	base := normalizeBaseURL(cfg.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/models", nil)
	if err != nil {
		return &Error{Kind: ErrKindBadRequest, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return &Error{Kind: ErrKindNetwork, Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return &Error{Kind: ErrKindAuth, Status: resp.StatusCode, Message: string(body)}
	}
	if resp.StatusCode >= 500 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return &Error{Kind: ErrKindUnknown, Status: resp.StatusCode, Message: string(body)}
	}
	return nil
}

// ---- Images API path ----

func (c *Client) callImages(ctx context.Context, base, apiKey string, p GenerateParams) ([]byte, error) {
	if len(p.ReferenceImages) > 0 {
		return c.callImagesEdits(ctx, base, apiKey, p)
	}
	return c.callImagesGenerations(ctx, base, apiKey, p)
}

func (c *Client) callImagesGenerations(ctx context.Context, base, apiKey string, p GenerateParams) ([]byte, error) {
	body, _ := json.Marshal(map[string]any{
		"model":   p.Model,
		"prompt":  p.Prompt,
		"size":    mapImageSize(p.Ratio),
		"quality": mapImageQuality(p.Pixels),
		"n":       1,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/images/generations", bytes.NewReader(body))
	if err != nil {
		return nil, &Error{Kind: ErrKindBadRequest, Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &Error{Kind: ErrKindNetwork, Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, classifyHTTPError(resp)
	}
	var parsed struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, &Error{Kind: ErrKindUnknown, Message: "decode: " + err.Error()}
	}
	if len(parsed.Data) == 0 || parsed.Data[0].B64JSON == "" {
		return nil, &Error{Kind: ErrKindUnknown, Message: "empty image payload"}
	}
	png, err := base64.StdEncoding.DecodeString(parsed.Data[0].B64JSON)
	if err != nil {
		return nil, &Error{Kind: ErrKindUnknown, Message: "base64: " + err.Error()}
	}
	return png, nil
}

func (c *Client) callImagesEdits(ctx context.Context, base, apiKey string, p GenerateParams) ([]byte, error) {
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	_ = mw.WriteField("model", p.Model)
	_ = mw.WriteField("prompt", p.Prompt)
	_ = mw.WriteField("size", mapImageSize(p.Ratio))
	_ = mw.WriteField("quality", mapImageQuality(p.Pixels))
	_ = mw.WriteField("n", "1")

	for i, ref := range p.ReferenceImages {
		mime, raw, err := decodeDataURL(ref.DataURL)
		if err != nil {
			return nil, &Error{Kind: ErrKindBadRequest, Message: fmt.Sprintf("ref[%d]: %v", i, err)}
		}
		filename := fmt.Sprintf("reference-%d.%s", i+1, mimeToExt(mime))
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename=%q`, filename))
		h.Set("Content-Type", mime)
		part, err := mw.CreatePart(h)
		if err != nil {
			return nil, &Error{Kind: ErrKindUnknown, Message: err.Error()}
		}
		if _, err := part.Write(raw); err != nil {
			return nil, &Error{Kind: ErrKindUnknown, Message: err.Error()}
		}
	}
	if err := mw.Close(); err != nil {
		return nil, &Error{Kind: ErrKindUnknown, Message: err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/images/edits", body)
	if err != nil {
		return nil, &Error{Kind: ErrKindBadRequest, Message: err.Error()}
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &Error{Kind: ErrKindNetwork, Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, classifyHTTPError(resp)
	}
	var parsed struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, &Error{Kind: ErrKindUnknown, Message: "decode: " + err.Error()}
	}
	if len(parsed.Data) == 0 || parsed.Data[0].B64JSON == "" {
		return nil, &Error{Kind: ErrKindUnknown, Message: "empty image payload"}
	}
	png, err := base64.StdEncoding.DecodeString(parsed.Data[0].B64JSON)
	if err != nil {
		return nil, &Error{Kind: ErrKindUnknown, Message: "base64: " + err.Error()}
	}
	return png, nil
}

// ---- Responses SSE path ----

func (c *Client) callResponses(ctx context.Context, base, apiKey string, p GenerateParams) ([]byte, error) {
	userContent := buildResponsesUserContent(p)
	body, _ := json.Marshal(map[string]any{
		"model": p.Model,
		"input": []map[string]any{
			{"role": "system", "content": systemPromptForCodex},
			{"role": "user", "content": userContent},
		},
		"tools":       []map[string]any{{"type": "image_generation", "output_format": "png"}},
		"tool_choice": map[string]any{"type": "image_generation"},
		"stream":      true,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/responses", bytes.NewReader(body))
	if err != nil {
		return nil, &Error{Kind: ErrKindBadRequest, Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "text/event-stream")
	// Codex-specific headers, copied from front-end implementation
	req.Header.Set("chatgpt-account-id", "")
	req.Header.Set("version", "0.122.0")
	req.Header.Set("originator", "codex_cli_rs")
	req.Header.Set("session_id", fmt.Sprintf("server-%d", time.Now().UnixNano()))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &Error{Kind: ErrKindNetwork, Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, classifyHTTPError(resp)
	}
	if resp.Body == nil {
		return nil, &Error{Kind: ErrKindUnknown, Message: "empty response body"}
	}

	sawImageEvent := false
	sawTextEvent := false
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, &Error{Kind: ErrKindNetwork, Message: err.Error()}
		}
		trim := strings.TrimSpace(line)
		if trim == "" {
			if err == io.EOF {
				break
			}
			continue
		}
		if strings.HasPrefix(trim, "event: ") {
			ev := strings.TrimSpace(trim[7:])
			if strings.HasPrefix(ev, sseEvtImageGen) {
				sawImageEvent = true
			}
			if strings.HasPrefix(ev, "response.output_text.") {
				sawTextEvent = true
			}
			continue
		}
		if strings.HasPrefix(trim, "data: ") {
			dataStr := strings.TrimSpace(trim[6:])
			if dataStr == "" || dataStr == "[DONE]" {
				continue
			}
			var obj any
			if jerr := json.Unmarshal([]byte(dataStr), &obj); jerr != nil {
				continue
			}
			if b64 := findResultBase64(obj); b64 != "" {
				png, derr := base64.StdEncoding.DecodeString(b64)
				if derr != nil {
					return nil, &Error{Kind: ErrKindUnknown, Message: "base64: " + derr.Error()}
				}
				return png, nil
			}
		}
		if err == io.EOF {
			break
		}
	}

	if !sawImageEvent && sawTextEvent {
		return nil, &Error{Kind: ErrKindBadRequest, Message: "模型未调用 image_generation 工具，请检查 prompt"}
	}
	return nil, &Error{Kind: ErrKindUnknown, Message: "SSE 流结束但未捕获到图片"}
}

// buildResponsesUserContent matches the front-end shape: text + optional input_image entries.
func buildResponsesUserContent(p GenerateParams) any {
	if len(p.ReferenceImages) == 0 {
		return fmt.Sprintf("请生成以下描述的图片:%s", p.Prompt)
	}
	parts := []map[string]any{
		{"type": "input_text", "text": fmt.Sprintf("请基于下方参考图生成新的图片,描述:%s", p.Prompt)},
	}
	for _, ref := range p.ReferenceImages {
		parts = append(parts, map[string]any{
			"type":      "input_image",
			"image_url": ref.DataURL,
		})
	}
	return parts
}

// findResultBase64 walks the SSE payload looking for a `result: <base64-png>` field.
func findResultBase64(obj any) string {
	switch v := obj.(type) {
	case map[string]any:
		for key, val := range v {
			if key == "result" {
				if s, ok := val.(string); ok && (strings.HasPrefix(s, "iVBOR") || len(s) > 1000) {
					return s
				}
			}
			if found := findResultBase64(val); found != "" {
				return found
			}
		}
	case []any:
		for _, item := range v {
			if found := findResultBase64(item); found != "" {
				return found
			}
		}
	}
	return ""
}

func classifyHTTPError(resp *http.Response) *Error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	msg := strings.TrimSpace(string(body))
	if len(msg) > 500 {
		msg = msg[:500] + "..."
	}
	kind := ErrKindUnknown
	switch resp.StatusCode {
	case 401, 403:
		kind = ErrKindAuth
	case 404:
		kind = ErrKindNotFound
	case 429:
		kind = ErrKindRateLimited
	case 400, 422:
		kind = ErrKindBadRequest
	}
	slog.Warn("upstream non-2xx",
		"status", resp.StatusCode,
		"kind", string(kind),
		"snippet", msg)
	return &Error{Kind: kind, Status: resp.StatusCode, Message: msg}
}

func decodeDataURL(d string) (mime string, raw []byte, err error) {
	const prefix = "data:"
	if !strings.HasPrefix(d, prefix) {
		return "", nil, errors.New("not a data url")
	}
	rest := d[len(prefix):]
	comma := strings.Index(rest, ",")
	if comma < 0 {
		return "", nil, errors.New("malformed data url")
	}
	meta := rest[:comma]
	payload := rest[comma+1:]
	parts := strings.Split(meta, ";")
	if len(parts) < 1 {
		return "", nil, errors.New("malformed data url meta")
	}
	mime = parts[0]
	raw, err = base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", nil, fmt.Errorf("base64: %w", err)
	}
	return mime, raw, nil
}

func mimeToExt(mime string) string {
	switch mime {
	case "image/jpeg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/webp":
		return "webp"
	default:
		if i := strings.Index(mime, "/"); i > 0 {
			return mime[i+1:]
		}
		return "bin"
	}
}

func normalizeBaseURL(u string) string {
	v := strings.TrimRight(strings.TrimSpace(u), "/")
	if strings.HasSuffix(v, "/v1") {
		v = v[:len(v)-3]
	}
	return v
}

func mapImageSize(ratio string) string {
	switch ratio {
	case "3:4", "9:16":
		return "1024x1536"
	case "4:3", "16:9":
		return "1536x1024"
	case "1:1":
		return "1024x1024"
	default:
		return "auto"
	}
}

func mapImageQuality(pixels string) string {
	switch pixels {
	case "4K":
		return "high"
	case "1K":
		return "low"
	default:
		return "medium"
	}
}
