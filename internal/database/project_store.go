package database

import (
	"context"
	"database/sql"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/services"
)

// ProjectStore backs the example projects feature. It wraps generated sqlc
// queries and maps rows into service-owned types. Delete it together with the
// rest of the projects example (see internal/server/project_handlers.go).
type ProjectStore struct {
	queries *db.Queries
}

var _ services.ProjectStore = (*ProjectStore)(nil)

func NewProjectStore(conn *sql.DB) *ProjectStore {
	return &ProjectStore{queries: db.New(conn)}
}

func (s *ProjectStore) CreateProject(ctx context.Context, userID int64, name string) (services.Project, error) {
	row, err := s.queries.CreateProject(ctx, db.CreateProjectParams{UserID: userID, Name: name})
	if err != nil {
		return services.Project{}, err
	}
	return toServiceProject(row), nil
}

func (s *ProjectStore) ListProjectsByUserID(ctx context.Context, userID int64) ([]services.Project, error) {
	rows, err := s.queries.ListProjectsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	projects := make([]services.Project, 0, len(rows))
	for _, row := range rows {
		projects = append(projects, toServiceProject(row))
	}
	return projects, nil
}

func (s *ProjectStore) DeleteProject(ctx context.Context, projectID, userID int64) (bool, error) {
	affected, err := s.queries.DeleteProjectByIDAndUserID(ctx, db.DeleteProjectByIDAndUserIDParams{
		ID:     projectID,
		UserID: userID,
	})
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func toServiceProject(row db.Project) services.Project {
	return services.Project{
		ID:        row.ID,
		UserID:    row.UserID,
		Name:      row.Name,
		CreatedAt: row.CreatedAt,
	}
}
