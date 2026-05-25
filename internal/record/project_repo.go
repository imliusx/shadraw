package record

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// ProjectRepository persists projects.
type ProjectRepository struct {
	db *gorm.DB
}

func NewProjectRepository(db *gorm.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

var projectColumns = []string{"id", "user_id", "name", "created_at", "updated_at"}

// ErrProjectNotFound mirrors ErrNotFound for projects.
var ErrProjectNotFound = errors.New("project not found")

// Create inserts a project owned by userID.
func (r *ProjectRepository) Create(ctx context.Context, p *Project) error {
	return r.db.WithContext(ctx).Select("user_id", "name").Create(p).Error
}

// List returns the user's projects ordered newest-first.
func (r *ProjectRepository) List(ctx context.Context, userID int64) ([]Project, error) {
	var out []Project
	err := r.db.WithContext(ctx).
		Select(projectColumns).
		Where("user_id = ?", userID).
		Order("id DESC").
		Find(&out).Error
	return out, err
}

// FindByID returns a project by id, scoped to userID.
func (r *ProjectRepository) FindByID(ctx context.Context, id, userID int64) (*Project, error) {
	var p Project
	err := r.db.WithContext(ctx).
		Select(projectColumns).
		Where("id = ? AND user_id = ?", id, userID).
		Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrProjectNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Rename updates a project's name.
func (r *ProjectRepository) Rename(ctx context.Context, id, userID int64, name string) error {
	res := r.db.WithContext(ctx).Model(&Project{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("name", name)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrProjectNotFound
	}
	return nil
}

// Delete removes a project (records auto-clear project_id via FK ON DELETE SET NULL).
func (r *ProjectRepository) Delete(ctx context.Context, id, userID int64) error {
	res := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&Project{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrProjectNotFound
	}
	return nil
}
