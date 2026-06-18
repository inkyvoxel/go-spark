package services

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeProjectStore struct {
	createdUserID int64
	createdName   string
	createErr     error
	listReturn    []Project
	deleteReturn  bool
	deletedID     int64
	deletedUserID int64
}

func (f *fakeProjectStore) CreateProject(_ context.Context, userID int64, name string) (Project, error) {
	f.createdUserID = userID
	f.createdName = name
	if f.createErr != nil {
		return Project{}, f.createErr
	}
	return Project{ID: 1, UserID: userID, Name: name}, nil
}

func (f *fakeProjectStore) ListProjectsByUserID(_ context.Context, _ int64) ([]Project, error) {
	return f.listReturn, nil
}

func (f *fakeProjectStore) DeleteProject(_ context.Context, projectID, userID int64) (bool, error) {
	f.deletedID = projectID
	f.deletedUserID = userID
	return f.deleteReturn, nil
}

func TestProjectServiceCreateTrimsAndStores(t *testing.T) {
	store := &fakeProjectStore{}
	svc := NewProjectService(store)

	project, err := svc.Create(context.Background(), 7, "  My Project  ")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if store.createdName != "My Project" {
		t.Fatalf("stored name = %q, want trimmed %q", store.createdName, "My Project")
	}
	if project.Name != "My Project" || project.UserID != 7 {
		t.Fatalf("project = %+v, want name trimmed and user 7", project)
	}
}

func TestProjectServiceCreateRejectsEmptyName(t *testing.T) {
	store := &fakeProjectStore{}
	svc := NewProjectService(store)

	if _, err := svc.Create(context.Background(), 7, "   "); !errors.Is(err, ErrProjectNameRequired) {
		t.Fatalf("Create() error = %v, want ErrProjectNameRequired", err)
	}
	if store.createdName != "" {
		t.Fatal("store should not be called when name is empty")
	}
}

func TestProjectServiceCreateRejectsTooLongName(t *testing.T) {
	store := &fakeProjectStore{}
	svc := NewProjectService(store)

	tooLong := strings.Repeat("a", ProjectNameMaxLength+1)
	if _, err := svc.Create(context.Background(), 7, tooLong); !errors.Is(err, ErrProjectNameTooLong) {
		t.Fatalf("Create() error = %v, want ErrProjectNameTooLong", err)
	}
}

func TestProjectServiceDeleteScopesToUserAndReportsMiss(t *testing.T) {
	store := &fakeProjectStore{deleteReturn: false}
	svc := NewProjectService(store)

	if err := svc.Delete(context.Background(), 5, 7); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("Delete() error = %v, want ErrProjectNotFound", err)
	}
	if store.deletedID != 5 || store.deletedUserID != 7 {
		t.Fatalf("delete scoped to (id=%d, user=%d), want (5, 7)", store.deletedID, store.deletedUserID)
	}
}

func TestProjectServiceDeleteSucceeds(t *testing.T) {
	store := &fakeProjectStore{deleteReturn: true}
	svc := NewProjectService(store)

	if err := svc.Delete(context.Background(), 5, 7); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
}
