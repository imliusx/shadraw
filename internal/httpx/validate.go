package httpx

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// BindJSON wraps gin.ShouldBindJSON, converting validation errors to a
// 422 response with a `fields` map. Returns false if the response has been
// written; caller should return immediately.
func BindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			FailWithFields(c, http.StatusUnprocessableEntity, CodeValidationFailed,
				"参数校验失败", validationFieldMap(ve))
			return false
		}
		Fail(c, http.StatusBadRequest, CodeValidationFailed, "请求体格式错误: "+err.Error())
		return false
	}
	return true
}

func validationFieldMap(ve validator.ValidationErrors) map[string]string {
	out := make(map[string]string, len(ve))
	for _, fe := range ve {
		key := lowerFirst(fe.Field())
		out[key] = humanizeTag(fe)
	}
	return out
}

func humanizeTag(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "必填"
	case "email":
		return "邮箱格式不合法"
	case "min":
		return "至少 " + fe.Param() + " 个字符"
	case "max":
		return "至多 " + fe.Param() + " 个字符"
	case "len":
		return "长度必须为 " + fe.Param()
	default:
		return "不符合规则: " + fe.Tag()
	}
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
