package httpx

// Error codes. The set is part of the public contract: only additions are
// allowed; renaming or removing requires a new API version.
const (
	CodeValidationFailed = "validation_failed"
	CodeUnauthorized     = "unauthorized"
	CodeForbidden        = "forbidden"
	CodeAccountDisabled  = "account_disabled"
	CodeNotFound         = "not_found"
	CodeConflict         = "conflict"
	CodeRateLimited      = "rate_limited"
	CodeUpstreamError    = "upstream_error"
	CodeInternalError    = "internal_error"
)
