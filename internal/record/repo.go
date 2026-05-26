package record

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrNotFound is returned when a record / project doesn't exist (or isn't yours).
var ErrNotFound = errors.New("record not found")

// ListParams filters list queries.
type ListParams struct {
	UserID    int64
	Status    string // optional
	ProjectID *int64 // optional; pointer to distinguish "no filter" from "null project"
	Favorite  *bool  // optional
	Page      int    // 1-based
	PageSize  int    // capped at 100
}

// Repository persists records.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// recordColumns is the projection used for SELECT — keeps queries explicit.
var recordColumns = []string{
	"id", "uuid", "user_id", "project_id", "prompt", "model", "image_params",
	"status", "favorite", "image_path", "error", "reference_images",
	"started_at", "completed_at", "created_at", "updated_at",
}

// Create inserts a new record. The DB default fills uuid; we read it back via RETURNING.
func (r *Repository) Create(ctx context.Context, rec *Record) error {
	return r.db.WithContext(ctx).
		Select("user_id", "project_id", "prompt", "model", "image_params", "status", "favorite", "reference_images").
		Create(rec).Error
}

// FindByID returns the record by id, scoped to userID unless userID == 0 (admin).
func (r *Repository) FindByID(ctx context.Context, id, userID int64) (*Record, error) {
	q := r.db.WithContext(ctx).Select(recordColumns).Where("id = ?", id)
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	var rec Record
	if err := q.Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &rec, nil
}

// List returns paginated records + total count.
func (r *Repository) List(ctx context.Context, p ListParams) ([]Record, int64, error) {
	q := r.db.WithContext(ctx).Model(&Record{}).Where("user_id = ?", p.UserID)
	if p.Status != "" {
		q = q.Where("status = ?", p.Status)
	}
	if p.ProjectID != nil {
		q = q.Where("project_id = ?", *p.ProjectID)
	}
	if p.Favorite != nil {
		q = q.Where("favorite = ?", *p.Favorite)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize <= 0 || p.PageSize > 100 {
		p.PageSize = 20
	}
	var out []Record
	err := q.Select(recordColumns).
		Order("id DESC").
		Offset((p.Page - 1) * p.PageSize).
		Limit(p.PageSize).
		Find(&out).Error
	if err != nil {
		return nil, 0, err
	}
	return out, total, err
}

// UpdateFavorite toggles favorite for a user's record.
func (r *Repository) UpdateFavorite(ctx context.Context, id, userID int64, favorite bool) error {
	res := r.db.WithContext(ctx).Model(&Record{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("favorite", favorite)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateProject moves a record to a project (or null).
func (r *Repository) UpdateProject(ctx context.Context, id, userID int64, projectID *int64) error {
	res := r.db.WithContext(ctx).Model(&Record{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("project_id", projectID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RetryFailed resets a failed record to waiting so the worker can process the
// same record again. Non-failed records are left unchanged and return ErrNotFound.
func (r *Repository) RetryFailed(ctx context.Context, id, userID int64) (*Record, error) {
	res := r.db.WithContext(ctx).Model(&Record{}).
		Where("id = ? AND user_id = ? AND status = ?", id, userID, StatusFailed).
		Updates(map[string]any{
			"status":       StatusWaiting,
			"error":        nil,
			"started_at":   nil,
			"completed_at": nil,
			"image_path":   nil,
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return r.FindByID(ctx, id, userID)
}

// Delete deletes a record scoped to userID (0 = admin, no scope).
func (r *Repository) Delete(ctx context.Context, id, userID int64) (*Record, error) {
	rec, err := r.FindByID(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Delete(&Record{}, rec.ID).Error; err != nil {
		return nil, err
	}
	return rec, nil
}

// ClaimWaiting atomically picks one waiting record and flips it to running.
// Returns ErrNotFound if no candidate exists.
//
// Uses Postgres' SELECT … FOR UPDATE SKIP LOCKED inside a transaction so
// multiple workers don't race on the same row.
func (r *Repository) ClaimWaiting(ctx context.Context) (*Record, error) {
	var picked Record
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Raw(`
			SELECT id FROM records
			WHERE status = 'waiting'
			ORDER BY id ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		`).Scan(&picked).Error
		if err != nil {
			return err
		}
		if picked.ID == 0 {
			return gorm.ErrRecordNotFound
		}
		now := time.Now()
		return tx.Model(&Record{}).
			Where("id = ?", picked.ID).
			Updates(map[string]any{
				"status":     StatusRunning,
				"started_at": now,
			}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	// re-read with full columns
	return r.FindByID(ctx, picked.ID, 0)
}

// StoreGenerated writes the success outcome for a single generated image.
func (r *Repository) StoreGenerated(ctx context.Context, id int64, imagePath string) error {
	if imagePath == "" {
		return errors.New("empty generated image path")
	}
	now := time.Now()
	return r.db.WithContext(ctx).Model(&Record{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       StatusCompleted,
			"image_path":   imagePath,
			"completed_at": now,
			"error":        nil,
		}).Error
}

// MarkFailed writes the failure outcome.
func (r *Repository) MarkFailed(ctx context.Context, id int64, msg string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&Record{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       StatusFailed,
			"error":        msg,
			"completed_at": now,
		}).Error
}

// SweepRunningToWaiting flips any record stuck in running back to waiting at boot.
func (r *Repository) SweepRunningToWaiting(ctx context.Context) (int64, error) {
	res := r.db.WithContext(ctx).Model(&Record{}).
		Where("status = ?", StatusRunning).
		Updates(map[string]any{
			"status":     StatusWaiting,
			"started_at": nil,
		})
	return res.RowsAffected, res.Error
}

// ---- admin helpers ----

// AdminList filters by status / userID for the admin overview.
type AdminListParams struct {
	Status   string
	UserID   *int64
	Page     int
	PageSize int
}

// AdminList returns global records (no user scoping).
func (r *Repository) AdminList(ctx context.Context, p AdminListParams) ([]Record, int64, error) {
	q := r.db.WithContext(ctx).Model(&Record{})
	if p.Status != "" {
		q = q.Where("status = ?", p.Status)
	}
	if p.UserID != nil {
		q = q.Where("user_id = ?", *p.UserID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize <= 0 || p.PageSize > 100 {
		p.PageSize = 20
	}
	var out []Record
	err := q.Select(recordColumns).
		Order("id DESC").
		Offset((p.Page - 1) * p.PageSize).
		Limit(p.PageSize).
		Find(&out).Error
	if err != nil {
		return nil, 0, err
	}
	return out, total, err
}

// AdminStatsOverview is the dashboard payload.
type AdminStatsOverview struct {
	Today struct {
		Total   int64 `json:"total"`
		Success int64 `json:"success"`
		Failed  int64 `json:"failed"`
		Running int64 `json:"running"`
		Waiting int64 `json:"waiting"`
		AvgMs   int64 `json:"avgMs"`
	} `json:"today"`
}

// StatsOverview aggregates today's record counts and average duration.
func (r *Repository) StatsOverview(ctx context.Context) (AdminStatsOverview, error) {
	var out AdminStatsOverview
	type row struct {
		Status string
		Count  int64
		AvgMs  *float64
	}
	var rows []row
	err := r.db.WithContext(ctx).Raw(`
		SELECT status,
		       COUNT(*) AS count,
		       AVG(EXTRACT(EPOCH FROM (completed_at - started_at)) * 1000) AS avg_ms
		FROM records
		WHERE created_at >= date_trunc('day', now())
		GROUP BY status
	`).Scan(&rows).Error
	if err != nil {
		return out, err
	}
	var sumMs float64
	var sumCount int64
	for _, r := range rows {
		out.Today.Total += r.Count
		switch r.Status {
		case string(StatusCompleted):
			out.Today.Success = r.Count
			if r.AvgMs != nil {
				sumMs += *r.AvgMs * float64(r.Count)
				sumCount += r.Count
			}
		case string(StatusFailed):
			out.Today.Failed = r.Count
		case string(StatusRunning):
			out.Today.Running = r.Count
		case string(StatusWaiting):
			out.Today.Waiting = r.Count
		}
	}
	if sumCount > 0 {
		out.Today.AvgMs = int64(sumMs / float64(sumCount))
	}
	return out, nil
}
