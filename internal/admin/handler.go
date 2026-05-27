package admin

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liusx/shadraw/internal/auth"
	"github.com/liusx/shadraw/internal/httpx"
	"github.com/liusx/shadraw/internal/imagegen"
	"github.com/liusx/shadraw/internal/record"
	"github.com/liusx/shadraw/internal/upstream"
	"github.com/liusx/shadraw/internal/user"
)

// Handler bundles admin endpoints.
type Handler struct {
	store      *Store
	users      *user.Repository
	records    *record.Repository
	upstreamCl *upstream.Client
	auth       AuthService
}

// AuthService is the subset of *auth.Service the admin handler uses.
type AuthService interface {
	ResetPasswordByAdmin(ctx context.Context, userID int64) (tempPassword string, err error)
}

func NewHandler(store *Store, users *user.Repository, records *record.Repository, upstreamCl *upstream.Client, authSvc AuthService) *Handler {
	return &Handler{store: store, users: users, records: records, upstreamCl: upstreamCl, auth: authSvc}
}

// ---- upstream config ----

func (h *Handler) GetUpstream(c *gin.Context) {
	httpx.OK(c, gin.H{"config": h.store.View()})
}

func (h *Handler) UpdateUpstream(c *gin.Context) {
	var req UpdateUpstreamReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	actorID, _ := strconv.ParseInt(httpx.UserIDFrom(c), 10, 64)
	if err := h.store.UpdateUpstream(c.Request.Context(), UpdateUpstreamArgs{
		BaseURL:       req.BaseURL,
		APIKey:        req.APIKey,
		EnabledModels: req.EnabledModels,
		ActorID:       actorID,
	}); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	httpx.OK(c, gin.H{"config": h.store.View()})
}

func (h *Handler) TestUpstream(c *gin.Context) {
	cfg := h.store.Snapshot()
	if cfg.BaseURL == "" || cfg.APIKey == "" {
		httpx.OK(c, TestConnectionResp{OK: false, Status: 0, Message: "未配置 baseUrl 或 apiKey"})
		return
	}

	// Optional body: if a model is supplied, do a real generation; otherwise
	// fall back to a cheap GET /v1/models probe.
	var req TestConnectionReq
	_ = c.ShouldBindJSON(&req) // body is optional

	if req.Model == "" {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		if err := h.upstreamCl.TestConnection(ctx, cfg); err != nil {
			resp := TestConnectionResp{OK: false}
			var ue *upstream.Error
			if errors.As(err, &ue) {
				resp.Status = ue.Status
				resp.Message = string(ue.Kind) + ": " + ue.Message
			} else {
				resp.Message = err.Error()
			}
			httpx.OK(c, resp)
			return
		}
		httpx.OK(c, TestConnectionResp{OK: true, Status: 200, Message: "/v1/models 可达"})
		return
	}

	// Validate model is in the admin whitelist (if one is configured).
	if enabled := h.store.EnabledModels(); len(enabled) > 0 {
		found := false
		for _, m := range enabled {
			if m == req.Model {
				found = true
				break
			}
		}
		if !found {
			httpx.OK(c, TestConnectionResp{
				OK: false, Status: 0,
				Message: "model 不在已启用列表中: " + req.Model,
			})
			return
		}
	}

	// Real image generation with a tiny prompt. 5 min upper bound matches worker.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	start := time.Now()
	result, err := h.upstreamCl.Generate(ctx, cfg, upstream.GenerateParams{
		Model:  req.Model,
		Prompt: "a small red circle on a white background, simple test",
		ImageParams: imagegen.Params{
			Size:    "1024x1024",
			Quality: imagegen.QualityLow,
		},
	})
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		resp := TestConnectionResp{OK: false, ElapsedMs: elapsed}
		var ue *upstream.Error
		if errors.As(err, &ue) {
			resp.Status = ue.Status
			resp.Message = string(ue.Kind) + ": " + ue.Message
		} else {
			resp.Message = err.Error()
		}
		httpx.OK(c, resp)
		return
	}
	httpx.OK(c, TestConnectionResp{
		OK:         true,
		Status:     200,
		Message:    "成功生成测试图片",
		ElapsedMs:  elapsed,
		ImageBytes: len(result.Image),
	})
}

// ---- runtime ----

func (h *Handler) GetRuntime(c *gin.Context) {
	httpx.OK(c, gin.H{"workerConcurrency": h.store.WorkerConcurrency()})
}

func (h *Handler) UpdateRuntime(c *gin.Context) {
	var req UpdateRuntimeReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	actorID, _ := strconv.ParseInt(httpx.UserIDFrom(c), 10, 64)
	if err := h.store.UpdateWorkerConcurrency(c.Request.Context(), req.WorkerConcurrency, actorID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	httpx.OK(c, gin.H{"workerConcurrency": h.store.WorkerConcurrency()})
}

// ---- site settings ----

func (h *Handler) GetSite(c *gin.Context) {
	httpx.OK(c, gin.H{"config": h.store.SiteConfig()})
}

func (h *Handler) UpdateSite(c *gin.Context) {
	var req UpdateSiteReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	actorID, _ := strconv.ParseInt(httpx.UserIDFrom(c), 10, 64)
	if err := h.store.UpdateSiteConfig(c.Request.Context(), req.SiteTitle, actorID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	httpx.OK(c, gin.H{"config": h.store.SiteConfig()})
}

// ---- users ----

func (h *Handler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	search := c.Query("search")

	users, total, err := h.users.AdminList(c.Request.Context(), user.AdminListParams{
		Search: search, Page: page, PageSize: pageSize,
	})
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	out := make([]auth.UserDTO, len(users))
	for i := range users {
		out[i] = auth.ToUserDTO(&users[i])
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages < 1 {
		totalPages = 1
	}
	httpx.OKWithMeta(c, gin.H{"users": out}, &httpx.Meta{
		Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages,
	})
}

func (h *Handler) UpdateUser(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req UpdateUserReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	if req.Disabled != nil {
		if err := h.users.SetDisabled(c.Request.Context(), id, *req.Disabled); err != nil {
			h.handleUserErr(c, err)
			return
		}
	}
	if req.Role != nil {
		if err := h.users.SetRole(c.Request.Context(), id, user.Role(*req.Role)); err != nil {
			h.handleUserErr(c, err)
			return
		}
	}
	u, err := h.users.FindByID(c.Request.Context(), id)
	if err != nil {
		h.handleUserErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"user": auth.ToUserDTO(u)})
}

func (h *Handler) ResetPassword(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	tmp, err := h.auth.ResetPasswordByAdmin(c.Request.Context(), id)
	if err != nil {
		h.handleUserErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"tempPassword": tmp})
}

// ---- records ----

func (h *Handler) ListRecords(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	params := record.AdminListParams{
		Status: c.Query("status"), Page: page, PageSize: pageSize,
	}
	if v := c.Query("userId"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			params.UserID = &n
		}
	}
	rows, total, err := h.records.AdminList(c.Request.Context(), params)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	out := make([]record.RecordDTO, len(rows))
	for i := range rows {
		out[i] = record.ToDTO(&rows[i])
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages < 1 {
		totalPages = 1
	}
	httpx.OKWithMeta(c, gin.H{"records": out}, &httpx.Meta{
		Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages,
	})
}

func (h *Handler) DeleteRecord(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	_, err := h.records.Delete(c.Request.Context(), id, 0) // admin: no user scoping
	if err != nil {
		if errors.Is(err, record.ErrNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "record 不存在")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// ---- stats ----

func (h *Handler) StatsOverview(c *gin.Context) {
	out, err := h.records.StatsOverview(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	httpx.OK(c, out)
}

// ---- helpers ----

func (h *Handler) handleUserErr(c *gin.Context, err error) {
	if errors.Is(err, user.ErrNotFound) {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "user 不存在")
		return
	}
	httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
}

func parseIDParam(c *gin.Context, key string) (int64, bool) {
	v := c.Param(key)
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "资源不存在")
		return 0, false
	}
	return n, true
}
