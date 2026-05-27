package record

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/liusx/shadraw/internal/blob"
	"github.com/liusx/shadraw/internal/httpx"
	"github.com/liusx/shadraw/internal/imagegen"
)

// UpstreamConfigReader exposes the enabled-models / connectivity required to
// validate record requests. The admin module owns the source of truth.
type UpstreamConfigReader interface {
	EnabledModels() []string
}

// Handler wires record + project + image endpoints.
type Handler struct {
	records  *Repository
	projects *ProjectRepository
	blob     blob.Store
	upstream UpstreamConfigReader
}

func NewHandler(records *Repository, projects *ProjectRepository, blobStore blob.Store, upstream UpstreamConfigReader) *Handler {
	return &Handler{records: records, projects: projects, blob: blobStore, upstream: upstream}
}

// ---- records ----

func (h *Handler) Create(c *gin.Context) {
	var req CreateRecordReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	userID := mustUserID(c)
	if userID == 0 {
		return
	}

	if enabled := h.upstream.EnabledModels(); len(enabled) > 0 && !contains(enabled, req.Model) {
		httpx.Fail(c, http.StatusUnprocessableEntity, httpx.CodeValidationFailed,
			"该模型未启用; 请联系管理员")
		return
	}

	var projID *int64
	if req.ProjectID != nil && *req.ProjectID != "" {
		id, err := strconv.ParseInt(*req.ProjectID, 10, 64)
		if err != nil {
			httpx.Fail(c, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, "projectId 非法")
			return
		}
		// Ownership check: a user can only attach records to their own projects.
		if _, perr := h.projects.FindByID(c.Request.Context(), id, userID); perr != nil {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "project 不存在")
			return
		}
		projID = &id
	}

	rec := &Record{
		UserID:          userID,
		ProjectID:       projID,
		Prompt:          req.Prompt,
		Model:           req.Model,
		ImageParams:     imagegen.Normalize(req.ImageParams),
		Status:          StatusWaiting,
		ReferenceImages: StringSlice(req.ReferenceImages),
	}
	if err := h.records.Create(c.Request.Context(), rec); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	// re-read so we get DB defaults (uuid, created_at)
	fresh, err := h.records.FindByID(c.Request.Context(), rec.ID, userID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	httpx.Created(c, gin.H{"record": ToDTO(fresh)})
}

func (h *Handler) Get(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	userID := mustUserID(c)
	if userID == 0 {
		return
	}
	rec, err := h.records.FindByID(c.Request.Context(), id, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "record 不存在")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	httpx.OK(c, gin.H{"record": ToDTO(rec)})
}

func (h *Handler) List(c *gin.Context) {
	userID := mustUserID(c)
	if userID == 0 {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	query := c.Query("q")
	if c.Query("scope") == "public" {
		rows, total, err := h.records.ListPublic(c.Request.Context(), userID, PublicListParams{
			Query:    query,
			Page:     page,
			PageSize: pageSize,
		})
		if err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
			return
		}
		out := make([]RecordDTO, len(rows))
		for i := range rows {
			out[i] = ToPublicDTO(&rows[i])
		}
		httpx.OKWithMeta(c, gin.H{"records": out}, pagingMeta(page, pageSize, total))
		return
	}
	p := ListParams{
		UserID:   userID,
		Status:   c.Query("status"),
		Query:    query,
		Page:     page,
		PageSize: pageSize,
	}
	if v := c.Query("projectId"); v != "" {
		if v == "none" {
			p.ProjectUnclassified = true
		} else {
			id, err := strconv.ParseInt(v, 10, 64)
			if err == nil {
				p.ProjectID = &id
			}
		}
	}
	if v := c.Query("favorite"); v == "true" || v == "false" {
		b := v == "true"
		p.Favorite = &b
	}
	rows, total, err := h.records.List(c.Request.Context(), p)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	out := make([]RecordDTO, len(rows))
	for i := range rows {
		out[i] = ToDTO(&rows[i])
	}
	meta := pagingMeta(p.Page, p.PageSize, total)
	httpx.OKWithMeta(c, gin.H{"records": out}, meta)
}

func (h *Handler) Update(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req UpdateRecordReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	userID := mustUserID(c)
	if userID == 0 {
		return
	}

	if req.Favorite != nil {
		if err := h.records.UpdateFavorite(c.Request.Context(), id, userID, *req.Favorite); err != nil {
			if errors.Is(err, ErrNotFound) {
				httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "record 不存在")
				return
			}
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
			return
		}
	}
	if req.IsPublic != nil {
		promptPublic := false
		if req.PromptPublic != nil {
			promptPublic = *req.PromptPublic
		}
		if err := h.records.UpdatePublic(c.Request.Context(), id, userID, *req.IsPublic, promptPublic); err != nil {
			if errors.Is(err, ErrNotFound) {
				httpx.Fail(c, http.StatusConflict, httpx.CodeConflict, "只有已完成的图片可以公开")
				return
			}
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
			return
		}
	}
	if req.ProjectID != nil {
		var projID *int64
		if *req.ProjectID != "" {
			n, err := strconv.ParseInt(*req.ProjectID, 10, 64)
			if err != nil {
				httpx.Fail(c, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, "projectId 非法")
				return
			}
			if _, perr := h.projects.FindByID(c.Request.Context(), n, userID); perr != nil {
				httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "project 不存在")
				return
			}
			projID = &n
		}
		if err := h.records.UpdateProject(c.Request.Context(), id, userID, projID); err != nil {
			if errors.Is(err, ErrNotFound) {
				httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "record 不存在")
				return
			}
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
			return
		}
	}
	rec, err := h.records.FindByID(c.Request.Context(), id, userID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	httpx.OK(c, gin.H{"record": ToDTO(rec)})
}

func (h *Handler) Retry(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	userID := mustUserID(c)
	if userID == 0 {
		return
	}
	rec, err := h.records.RetryFailed(c.Request.Context(), id, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Fail(c, http.StatusConflict, httpx.CodeConflict, "只能重试失败的记录")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	httpx.OK(c, gin.H{"record": ToDTO(rec)})
}

func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	userID := mustUserID(c)
	if userID == 0 {
		return
	}
	rec, err := h.records.Delete(c.Request.Context(), id, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "record 不存在")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	if rec.ImagePath != nil && *rec.ImagePath != "" {
		_ = h.blob.Delete(c.Request.Context(), *rec.ImagePath)
	}
	httpx.OK(c, gin.H{"ok": true})
}

// StreamImage returns the binary image bytes for a record the caller owns.
func (h *Handler) StreamImage(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	userID := mustUserID(c)
	if userID == 0 {
		return
	}
	rec, err := h.records.FindVisibleImageByID(c.Request.Context(), id, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "record 不存在")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	if rec.ImagePath == nil || *rec.ImagePath == "" {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "image not ready")
		return
	}
	var buf bytes.Buffer
	if gerr := h.blob.Get(c.Request.Context(), *rec.ImagePath, &buf); gerr != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "image not found")
		return
	}
	mime := imagegen.MIME(imagegen.Normalize(&rec.ImageParams).OutputFormat)
	c.Header("Content-Type", mime)
	c.Header("Cache-Control", "private, max-age=86400")
	c.Data(http.StatusOK, mime, buf.Bytes())
}

// ---- projects ----

func (h *Handler) CreateProject(c *gin.Context) {
	var req CreateProjectReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	userID := mustUserID(c)
	if userID == 0 {
		return
	}
	p := &Project{UserID: userID, Name: req.Name}
	if err := h.projects.Create(c.Request.Context(), p); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	fresh, err := h.projects.FindByID(c.Request.Context(), p.ID, userID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	httpx.Created(c, gin.H{"project": ToProjectDTO(fresh)})
}

func (h *Handler) ListProjects(c *gin.Context) {
	userID := mustUserID(c)
	if userID == 0 {
		return
	}
	rows, err := h.projects.List(c.Request.Context(), userID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	out := make([]ProjectDTO, len(rows))
	for i := range rows {
		out[i] = ToProjectDTO(&rows[i])
	}
	httpx.OK(c, gin.H{"projects": out})
}

func (h *Handler) RenameProject(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req RenameProjectReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	userID := mustUserID(c)
	if userID == 0 {
		return
	}
	if err := h.projects.Rename(c.Request.Context(), id, userID, req.Name); err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "project 不存在")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) DeleteProject(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	userID := mustUserID(c)
	if userID == 0 {
		return
	}
	if err := h.projects.Delete(c.Request.Context(), id, userID); err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "project 不存在")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternalError, "internal error")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// ---- helpers ----

func mustUserID(c *gin.Context) int64 {
	uid, err := strconv.ParseInt(httpx.UserIDFrom(c), 10, 64)
	if err != nil || uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "无效会话")
		return 0
	}
	return uid
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

func pagingMeta(page, pageSize int, total int64) *httpx.Meta {
	if pageSize <= 0 {
		pageSize = 20
	}
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages < 1 {
		totalPages = 1
	}
	return &httpx.Meta{
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
