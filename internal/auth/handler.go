package auth

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/liusx/shadraw/internal/httpx"
	"github.com/liusx/shadraw/internal/user"
)

// Handler bundles the auth HTTP endpoints.
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Exported handlers — main wires each route individually so it can attach
// per-route middleware (rate limit, require auth) as the spec dictates.
func (h *Handler) RegisterEndpoint(c *gin.Context)       { h.register(c) }
func (h *Handler) LoginEndpoint(c *gin.Context)          { h.login(c) }
func (h *Handler) RefreshEndpoint(c *gin.Context)        { h.refresh(c) }
func (h *Handler) LogoutEndpoint(c *gin.Context)         { h.logout(c) }
func (h *Handler) MeEndpoint(c *gin.Context)             { h.me(c) }
func (h *Handler) ChangePasswordEndpoint(c *gin.Context) { h.changePassword(c) }

func (h *Handler) register(c *gin.Context) {
	var req RegisterReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	resp, err := h.svc.Register(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrEmailTaken):
			httpx.Fail(c, http.StatusConflict, httpx.CodeConflict, "邮箱已被注册")
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		}
		return
	}
	httpx.Created(c, resp)
}

func (h *Handler) login(c *gin.Context) {
	var req LoginReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	resp, err := h.svc.Login(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "邮箱或密码错误")
		case errors.Is(err, ErrUserDisabled):
			httpx.Fail(c, http.StatusForbidden, httpx.CodeAccountDisabled, "账号已禁用")
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		}
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) refresh(c *gin.Context) {
	var req RefreshReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	pair, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		switch {
		case errors.Is(err, ErrRefreshInvalid),
			errors.Is(err, ErrRefreshExpired),
			errors.Is(err, ErrRefreshRevoked):
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "refresh token 无效")
		case errors.Is(err, ErrUserDisabled):
			httpx.Fail(c, http.StatusForbidden, httpx.CodeAccountDisabled, "账号已禁用")
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		}
		return
	}
	httpx.OK(c, gin.H{"tokens": pair})
}

func (h *Handler) logout(c *gin.Context) {
	var req LogoutReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) me(c *gin.Context) {
	uid, err := strconv.ParseInt(httpx.UserIDFrom(c), 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "无效会话")
		return
	}
	u, err := h.svc.Me(c.Request.Context(), uid)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "用户不存在")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	httpx.OK(c, gin.H{"user": ToUserDTO(u)})
}

func (h *Handler) changePassword(c *gin.Context) {
	var req ChangePasswordReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	uid, err := strconv.ParseInt(httpx.UserIDFrom(c), 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "无效会话")
		return
	}
	if err := h.svc.ChangePassword(c.Request.Context(), uid, req.OldPassword, req.NewPassword); err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "旧密码错误")
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		}
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}
