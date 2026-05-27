package record

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/liusx/shadraw/internal/imagegen"
)

func TestToPublicDTO_HidesPromptWhenPromptIsPrivate(t *testing.T) {
	projectID := int64(9)
	rec := &Record{
		ID:           1,
		UUID:         "00000000-0000-0000-0000-000000000001",
		ProjectID:    &projectID,
		Prompt:       "private prompt",
		Model:        "gpt-image-2",
		ImageParams:  imagegen.Params{Size: "1024x1024", Quality: imagegen.QualityHigh},
		Status:       StatusCompleted,
		IsPublic:     true,
		PromptPublic: false,
		CreatedAt:    time.Unix(0, 0).UTC(),
	}

	dto := ToPublicDTO(rec)

	if dto.Prompt != "" {
		t.Fatalf("Prompt = %q, want hidden", dto.Prompt)
	}
	if dto.ProjectID != "" {
		t.Fatalf("ProjectID = %q, want hidden", dto.ProjectID)
	}
	if dto.PromptPublic {
		t.Fatalf("PromptPublic = true, want false")
	}
}

func TestToPublicDTO_KeepsPromptWhenPromptIsPublic(t *testing.T) {
	rec := &Record{
		ID:           2,
		UUID:         "00000000-0000-0000-0000-000000000002",
		Prompt:       "public prompt",
		Model:        "gpt-image-2",
		ImageParams:  imagegen.Params{Size: "1024x1024", Quality: imagegen.QualityHigh},
		Status:       StatusCompleted,
		IsPublic:     true,
		PromptPublic: true,
		CreatedAt:    time.Unix(0, 0).UTC(),
	}

	dto := ToPublicDTO(rec)

	if dto.Prompt != rec.Prompt {
		t.Fatalf("Prompt = %q, want %q", dto.Prompt, rec.Prompt)
	}
	if !dto.PromptPublic {
		t.Fatalf("PromptPublic = false, want true")
	}
}

func TestUpdateRecordReq_OmittedPromptPublicIsNil(t *testing.T) {
	var req UpdateRecordReq
	if err := json.Unmarshal([]byte(`{"isPublic":true}`), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.IsPublic == nil || !*req.IsPublic {
		t.Fatalf("IsPublic = %v, want true pointer", req.IsPublic)
	}
	if req.PromptPublic != nil {
		t.Fatalf("PromptPublic = %v, want nil when omitted", *req.PromptPublic)
	}
}
