package blob

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

func TestS3Key_JoinsPathSegments(t *testing.T) {
	got := s3Key("images", "user-1", "abc.png")
	if got != "images/user-1/abc.png" {
		t.Fatalf("s3Key = %q", got)
	}
}

func TestMapS3Error_NotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "modeled no such key", err: &types.NoSuchKey{}},
		{name: "generic not found", err: &smithy.GenericAPIError{Code: "NotFound"}},
		{name: "generic 404", err: &smithy.GenericAPIError{Code: "404"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := mapS3Error(tt.err); !errors.Is(err, ErrNotFound) {
				t.Fatalf("mapS3Error() = %v, want ErrNotFound", err)
			}
		})
	}
}
