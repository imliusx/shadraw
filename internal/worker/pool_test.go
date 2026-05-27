package worker

import (
	"testing"

	"github.com/liusx/shadraw/internal/upstream"
)

func TestUserFacingGenerationErrorSafetyRejection(t *testing.T) {
	err := &upstream.Error{
		Kind:    upstream.ErrKindBadRequest,
		Status:  400,
		Message: "image_generation_user_error: Your request was rejected by the safety system. code=moderation_blocked",
	}

	got := userFacingGenerationError(err)
	want := "提示词被安全系统拒绝，请调整提示词后重试"
	if got != want {
		t.Fatalf("userFacingGenerationError() = %q, want %q", got, want)
	}
}

func TestUserFacingGenerationErrorBadRequestFallback(t *testing.T) {
	err := &upstream.Error{
		Kind:    upstream.ErrKindBadRequest,
		Status:  400,
		Message: "invalid size",
	}

	got := userFacingGenerationError(err)
	want := "生成请求被上游拒绝，请查看详情后重试"
	if got != want {
		t.Fatalf("userFacingGenerationError() = %q, want %q", got, want)
	}
}
