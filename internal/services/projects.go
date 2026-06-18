package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// The projects feature is an example, included to show the full
// path -> route -> handler -> service -> store -> SQL layering described in
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
type Project struct {
	ID        int64
	UserID    int64
	Name      string
	CreatedAt time.Time
}

// ProjectStore is the persistence the service needs. internal/database
// implements it; the service depends on this interface, not on generated code.
type ProjectStore interface {
	CreateProject(ctx context.Context, userID int64, name string) (Project, error)
	ListProjectsByUserID(ctx context.Context, userID int64) ([]Project, error)
	DeleteProject(ctx context.Context, projectID, userID int64) (bool, error)
}

type ProjectService struct {
	store ProjectStore
}

func NewProjectService(store ProjectStore) *ProjectService {
	return &ProjectService{store: store}
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

	project, err := s.store.CreateProject(ctx, userID, name)
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return project, nil
}

// List returns the user's projects, newest first.
func (s *ProjectService) List(ctx context.Context, userID int64) ([]Project, error) {
	projects, err := s.store.ListProjectsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return projects, nil
}

// Delete removes a project the user owns. The user ID scopes the delete so one
// user cannot delete another's project; a miss returns ErrProjectNotFound.
func (s *ProjectService) Delete(ctx context.Context, projectID, userID int64) error {
	deleted, err := s.store.DeleteProject(ctx, projectID, userID)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if !deleted {
		return ErrProjectNotFound
	}
	return nil
}
