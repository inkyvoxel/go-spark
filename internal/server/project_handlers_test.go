package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/inkyvoxel/go-spark/internal/paths"
	"github.com/inkyvoxel/go-spark/internal/services"
)

type fakeProjectService struct {
	createName   string
	createReturn services.Project
	createErr    error
	listReturn   []services.Project
	listErr      error
	deletedID    int64
	deleteErr    error
}

func (f *fakeProjectService) Create(_ context.Context, userID int64, name string) (services.Project, error) {
	f.createName = name
	if f.createErr != nil {
		return services.Project{}, f.createErr
	}
	return services.Project{ID: 1, UserID: userID, Name: name}, nil
}

func (f *fakeProjectService) List(_ context.Context, _ int64) ([]services.Project, error) {
	return f.listReturn, f.listErr
}

func (f *fakeProjectService) Delete(_ context.Context, projectID, _ int64) error {
	f.deletedID = projectID
	return f.deleteErr
}

func newProjectTestServer(t *testing.T, projects projectService) *Server {
	t.Helper()

	return &Server{
		projects:      projects,
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		postOnlyPaths: make(map[string]struct{}),
		templates: testTemplates(t, map[string]string{
			templateProjects: `projects {{ .Error }} {{ with index .FieldErrors "name" }}name-err={{ . }}{{ end }} {{ range .Projects }}p={{ .ID }}:{{ .Name }} {{ end }}`,
		}),
	}
}

func projectRequest(t *testing.T, method, target string, form url.Values) *http.Request {
	t.Helper()

	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	user := services.User{ID: 7, Email: "user@example.com"}
	return req.WithContext(contextWithUser(req.Context(), user))
}

func TestProjectsIndexRendersUserProjects(t *testing.T) {
	svc := &fakeProjectService{listReturn: []services.Project{{ID: 3, Name: "Alpha"}}}
	srv := newProjectTestServer(t, svc)

	rec := httptest.NewRecorder()
	srv.projectsIndex(rec, projectRequest(t, http.MethodGet, paths.Projects, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "p=3:Alpha") {
		t.Fatalf("body = %q, want rendered project", rec.Body.String())
	}
}

func TestCreateProjectRedirectsOnSuccess(t *testing.T) {
	svc := &fakeProjectService{}
	srv := newProjectTestServer(t, svc)

	rec := httptest.NewRecorder()
	srv.createProject(rec, projectRequest(t, http.MethodPost, paths.Projects, url.Values{"name": {"My Project"}}))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != paths.Projects {
		t.Fatalf("Location = %q, want %q", loc, paths.Projects)
	}
	if svc.createName != "My Project" {
		t.Fatalf("created name = %q, want %q", svc.createName, "My Project")
	}
}

func TestCreateProjectShowsFieldErrorOnInvalidName(t *testing.T) {
	svc := &fakeProjectService{createErr: services.ErrProjectNameRequired}
	srv := newProjectTestServer(t, svc)

	rec := httptest.NewRecorder()
	srv.createProject(rec, projectRequest(t, http.MethodPost, paths.Projects, url.Values{"name": {""}}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if !strings.Contains(rec.Body.String(), "name-err=") {
		t.Fatalf("body = %q, want a name field error", rec.Body.String())
	}
}

func TestDeleteProjectRedirectsOnSuccess(t *testing.T) {
	svc := &fakeProjectService{}
	srv := newProjectTestServer(t, svc)

	rec := httptest.NewRecorder()
	srv.deleteProject(rec, projectRequest(t, http.MethodPost, paths.ProjectsDelete, url.Values{"project_id": {"5"}}))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if svc.deletedID != 5 {
		t.Fatalf("deleted id = %d, want 5", svc.deletedID)
	}
}

func TestDeleteProjectRejectsInvalidID(t *testing.T) {
	svc := &fakeProjectService{}
	srv := newProjectTestServer(t, svc)

	rec := httptest.NewRecorder()
	srv.deleteProject(rec, projectRequest(t, http.MethodPost, paths.ProjectsDelete, url.Values{"project_id": {"not-a-number"}}))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d (flash + redirect)", rec.Code, http.StatusSeeOther)
	}
	if svc.deletedID != 0 {
		t.Fatal("Delete should not be called for an invalid id")
	}
}
