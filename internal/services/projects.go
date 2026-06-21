package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
)

// The projects feature is an example, included to show the
// path -> route -> handler -> service -> SQL layering described in
// docs/extending.md. Copy it as a starting point for your own features, or
// delete it (see the header in internal/server/project_handlers.go for the
// complete list of files to remove).

// ProjectNameMaxLength bounds a project name so a single field cannot store an
// unbounded blob.
const ProjectNameMaxLength = 100

var (
	ErrProjectNameRequired = errors.New("project name required")
	ErrProjectNameTooLong  = errors.New("project name too long")
	ErrProjectNotFound     = errors.New("project not found")
)

// Project is the service-owned representation of a row in the projects table.
// Handlers and templates see this type, never the generated db row.
type Project struct {
	ID        int64
	UserID    int64
	Name      string
	CreatedAt time.Time
}

// ProjectService owns both the business rules and the SQL for projects. It
// calls the sqlc-generated queries directly; there is no separate store layer.
type ProjectService struct {
	queries *db.Queries
}

func NewProjectService(conn *sql.DB) *ProjectService {
	return &ProjectService{queries: db.New(conn)}
}

// Create validates the name and stores a new project owned by the user.
func (s *ProjectService) Create(ctx context.Context, userID int64, name string) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, ErrProjectNameRequired
	}
	if utf8.RuneCountInString(name) > ProjectNameMaxLength {
		return Project{}, ErrProjectNameTooLong
	}

	row, err := s.queries.CreateProject(ctx, db.CreateProjectParams{UserID: userID, Name: name})
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return toProject(row), nil
}

// List returns the user's projects, newest first.
func (s *ProjectService) List(ctx context.Context, userID int64) ([]Project, error) {
	rows, err := s.queries.ListProjectsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	projects := make([]Project, 0, len(rows))
	for _, row := range rows {
		projects = append(projects, toProject(row))
	}
	return projects, nil
}

// Delete removes a project the user owns. The user ID scopes the delete so one
// user cannot delete another's project; a miss returns ErrProjectNotFound.
func (s *ProjectService) Delete(ctx context.Context, projectID, userID int64) error {
	affected, err := s.queries.DeleteProjectByIDAndUserID(ctx, db.DeleteProjectByIDAndUserIDParams{
		ID:     projectID,
		UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if affected == 0 {
		return ErrProjectNotFound
	}
	return nil
}

func toProject(row db.Project) Project {
	return Project{
		ID:        row.ID,
		UserID:    row.UserID,
		Name:      row.Name,
		CreatedAt: row.CreatedAt,
	}
}
