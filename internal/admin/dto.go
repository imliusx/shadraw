package admin

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// JSONArray adapts a Go []string to a Postgres JSONB column.
type JSONArray []string

func (a JSONArray) Value() (driver.Value, error) {
	if a == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]string(a))
}

func (a *JSONArray) Scan(src any) error {
	if src == nil {
		*a = JSONArray{}
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return errors.New("JSONArray.Scan: unsupported")
	}
	if len(b) == 0 {
		*a = JSONArray{}
		return nil
	}
	return json.Unmarshal(b, a)
}

// UpstreamConfigDTO is the public shape (with masked apiKey).
type UpstreamConfigDTO struct {
	BaseURL           string   `json:"baseUrl"`
	APIKeyMasked      string   `json:"apiKeyMasked,omitempty"`
	APIKeySet         bool     `json:"apiKeySet"`
	EnabledModels     []string `json:"enabledModels"`
	WorkerConcurrency int      `json:"workerConcurrency"`
}

// AppConfigDTO is the public app config used by the front-end before and after login.
type AppConfigDTO struct {
	EnabledModels []string `json:"enabledModels"`
	SiteTitle     string   `json:"siteTitle"`
}

// SiteConfigDTO is the admin-editable site settings shape.
type SiteConfigDTO struct {
	SiteTitle string `json:"siteTitle"`
}

// UpdateUpstreamReq is the body of PUT /api/v1/admin/upstream-configs.
type UpdateUpstreamReq struct {
	BaseURL       string   `json:"baseUrl" binding:"omitempty,max=512"`
	APIKey        *string  `json:"apiKey,omitempty"` // nil = unchanged; "" = clear; "***...***" = unchanged
	EnabledModels []string `json:"enabledModels" binding:"omitempty,dive,max=64"`
}

// UpdateSiteReq is the body of PATCH /api/v1/admin/site-settings.
type UpdateSiteReq struct {
	SiteTitle string `json:"siteTitle" binding:"required,min=1,max=64"`
}

// UpdateRuntimeReq is the body of PATCH /api/v1/admin/runtime.
type UpdateRuntimeReq struct {
	WorkerConcurrency int `json:"workerConcurrency" binding:"required,min=1,max=16"`
}

// UpdateUserReq is the body of PATCH /api/v1/admin/users/:id.
type UpdateUserReq struct {
	Disabled *bool   `json:"disabled,omitempty"`
	Role     *string `json:"role,omitempty" binding:"omitempty,oneof=admin user"`
}

// TestConnectionReq is the optional body of POST /upstream-configs/test.
// If Model is supplied, the backend actually calls the upstream image API
// with that model; otherwise it falls back to GET /v1/models.
type TestConnectionReq struct {
	Model string `json:"model"`
}

// TestConnectionResp is the response of POST /api/v1/admin/upstream-configs/test.
type TestConnectionResp struct {
	OK         bool   `json:"ok"`
	Status     int    `json:"status"`
	Message    string `json:"message,omitempty"`
	ElapsedMs  int64  `json:"elapsedMs,omitempty"`
	ImageBytes int    `json:"imageBytes,omitempty"`
}
