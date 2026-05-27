package record

import (
	"strconv"
	"time"

	"github.com/liusx/shadraw/internal/imagegen"
)

// CreateRecordReq is the body of POST /api/v1/records.
type CreateRecordReq struct {
	Prompt          string           `json:"prompt" binding:"required,min=1,max=4096"`
	Model           string           `json:"model" binding:"required,max=64"`
	ImageParams     *imagegen.Params `json:"imageParams"`
	ProjectID       *string          `json:"projectId"` // string-encoded int64; optional
	ReferenceImages []string         `json:"referenceImages" binding:"omitempty,max=4,dive,startswith=data:image/"`
}

// UpdateRecordReq is the body of PATCH /api/v1/records/:id.
type UpdateRecordReq struct {
	Favorite     *bool   `json:"favorite,omitempty"`
	IsPublic     *bool   `json:"isPublic,omitempty"`
	PromptPublic *bool   `json:"promptPublic,omitempty"`
	ProjectID    *string `json:"projectId,omitempty"` // "" = clear; non-empty = move; absent = unchanged
}

// CreateProjectReq is the body of POST /api/v1/projects.
type CreateProjectReq struct {
	Name string `json:"name" binding:"required,min=1,max=64"`
}

// RenameProjectReq is the body of PATCH /api/v1/projects/:id.
type RenameProjectReq struct {
	Name string `json:"name" binding:"required,min=1,max=64"`
}

// RecordDTO is the public shape.
type RecordDTO struct {
	ID             string          `json:"id"`
	UUID           string          `json:"uuid"`
	Prompt         string          `json:"prompt"`
	Model          string          `json:"model"`
	ImageParams    imagegen.Params `json:"imageParams"`
	Status         string          `json:"status"`
	Favorite       bool            `json:"favorite"`
	IsPublic       bool            `json:"isPublic"`
	PromptPublic   bool            `json:"promptPublic"`
	HasImage       bool            `json:"hasImage"`
	Error          string          `json:"error,omitempty"`
	UpstreamError  string          `json:"upstreamError,omitempty"`
	ProjectID      string          `json:"projectId,omitempty"`
	ReferenceCount int             `json:"referenceCount"`
	StartedAt      string          `json:"startedAt,omitempty"`
	CompletedAt    string          `json:"completedAt,omitempty"`
	PublishedAt    string          `json:"publishedAt,omitempty"`
	CreatedAt      string          `json:"createdAt"`
}

// ToDTO converts a Record to its public shape.
func ToDTO(r *Record) RecordDTO {
	hasImage := r.ImagePath != nil && *r.ImagePath != ""
	out := RecordDTO{
		ID:             strconv.FormatInt(r.ID, 10),
		UUID:           r.UUID,
		Prompt:         r.Prompt,
		Model:          r.Model,
		ImageParams:    imagegen.Normalize(&r.ImageParams),
		Status:         string(r.Status),
		Favorite:       r.Favorite,
		IsPublic:       r.IsPublic,
		PromptPublic:   r.PromptPublic,
		HasImage:       hasImage,
		ReferenceCount: len(r.ReferenceImages),
		CreatedAt:      r.CreatedAt.UTC().Format(time.RFC3339),
	}
	if r.Error != nil {
		out.Error = *r.Error
	}
	if r.UpstreamError != nil {
		out.UpstreamError = *r.UpstreamError
	}
	if r.ProjectID != nil {
		out.ProjectID = strconv.FormatInt(*r.ProjectID, 10)
	}
	if r.StartedAt != nil {
		out.StartedAt = r.StartedAt.UTC().Format(time.RFC3339)
	}
	if r.CompletedAt != nil {
		out.CompletedAt = r.CompletedAt.UTC().Format(time.RFC3339)
	}
	if r.PublishedAt != nil {
		out.PublishedAt = r.PublishedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// ToPublicDTO converts a record for the community gallery. Private project
// membership is hidden, and prompt text is omitted unless explicitly public.
func ToPublicDTO(r *Record) RecordDTO {
	out := ToDTO(r)
	out.ProjectID = ""
	out.UpstreamError = ""
	if !out.PromptPublic {
		out.Prompt = ""
	}
	return out
}

// ProjectDTO is the public shape.
type ProjectDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

// ToProjectDTO converts a Project to its public shape.
func ToProjectDTO(p *Project) ProjectDTO {
	return ProjectDTO{
		ID:        strconv.FormatInt(p.ID, 10),
		Name:      p.Name,
		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
	}
}
