package imagegen

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

type Background string

const (
	BackgroundAuto        Background = "auto"
	BackgroundTransparent Background = "transparent"
	BackgroundOpaque      Background = "opaque"
)

type Moderation string

const (
	ModerationAuto Moderation = "auto"
	ModerationLow  Moderation = "low"
)

type OutputFormat string

const (
	OutputFormatPNG  OutputFormat = "png"
	OutputFormatJPEG OutputFormat = "jpeg"
	OutputFormatWebP OutputFormat = "webp"
)

type Quality string

const (
	QualityAuto   Quality = "auto"
	QualityHigh   Quality = "high"
	QualityMedium Quality = "medium"
	QualityLow    Quality = "low"
)

// Params mirrors OpenAI image generation parameters that are useful for both
// /v1/images and the Responses image_generation tool.
type Params struct {
	Size              string       `json:"size,omitempty"`
	Quality           Quality      `json:"quality,omitempty"`
	N                 int          `json:"n,omitempty"`
	Background        Background   `json:"background,omitempty"`
	Moderation        Moderation   `json:"moderation,omitempty"`
	OutputFormat      OutputFormat `json:"output_format,omitempty"`
	OutputCompression *int         `json:"output_compression,omitempty"`
	Stream            *bool        `json:"stream,omitempty"`
	PartialImages     *int         `json:"partial_images,omitempty"`
	InputFidelity     string       `json:"input_fidelity,omitempty"`
	Mask              string       `json:"mask,omitempty"`
	ResponseFormat    string       `json:"response_format,omitempty"`
	Style             string       `json:"style,omitempty"`
	User              string       `json:"user,omitempty"`
}

func DefaultParams() Params {
	return Params{
		Size:         "auto",
		Quality:      QualityAuto,
		N:            1,
		Background:   BackgroundAuto,
		Moderation:   ModerationAuto,
		OutputFormat: OutputFormatPNG,
	}
}

func Normalize(params *Params) Params {
	if params == nil {
		return DefaultParams()
	}

	out := *params
	defaults := DefaultParams()
	if out.Size == "" {
		out.Size = defaults.Size
	}
	if out.Quality == "" {
		out.Quality = defaults.Quality
	}
	// The product currently supports one image per record. Keep the official
	// `n` field in the parameter shape, but never pass multi-image requests
	// through to upstream.
	out.N = defaults.N
	if out.Background == "" {
		out.Background = defaults.Background
	}
	if out.Moderation == "" {
		out.Moderation = defaults.Moderation
	}
	if out.OutputFormat == "" {
		out.OutputFormat = defaults.OutputFormat
	}
	return out
}

func Extension(format OutputFormat) string {
	switch format {
	case OutputFormatJPEG:
		return "jpg"
	case OutputFormatWebP:
		return "webp"
	default:
		return "png"
	}
}

func MIME(format OutputFormat) string {
	switch format {
	case OutputFormatJPEG:
		return "image/jpeg"
	case OutputFormatWebP:
		return "image/webp"
	default:
		return "image/png"
	}
}

func (p Params) Value() (driver.Value, error) {
	return json.Marshal(p)
}

func (p *Params) Scan(src any) error {
	if src == nil {
		*p = Params{}
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return errors.New("Params.Scan: unsupported type")
	}
	return json.Unmarshal(b, p)
}
