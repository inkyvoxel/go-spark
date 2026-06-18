package server

// Example feature: a minimal, user-owned "projects" list (create / list /
// delete). It exists to demonstrate the full layering documented in
// docs/extending.md, and is intentionally self-contained so you can copy it for
// your own features or delete it cleanly.
//
// To remove the example, delete these files:
//   - internal/server/project_handlers.go        (this file) + _test.go
//   - internal/services/projects.go               + _test.go
//   - internal/database/project_store.go          + _test.go
//   - internal/db/queries/projects.sql            (then run `make sqlc`)
//   - migrations/00002_projects_schema.sql
//   - templates/projects/index.html
// and remove the small wiring in each of:
//   - internal/paths/paths.go                     (Projects/ProjectsDelete + map entries)
//   - internal/server/template_constants.go       (templateProjects)
//   - internal/server/server.go                   (parseTemplates entry, Server/Options
//                                                   field, New assignment, registerProjectRoutes)
//   - internal/server/auth_handlers.go            (templateData.Projects field)
//   - internal/app/build.go                       (store + service wiring)
//   - templates/layout.html                       (the Projects nav link)

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/inkyvoxel/go-spark/internal/paths"
	"github.com/inkyvoxel/go-spark/internal/services"
)

// projectService is the slice of the projects service the handlers need. Keeping
// it as a local interface keeps handlers away from database details.
type projectService interface {
	Create(ctx context.Context, userID int64, name string) (services.Project, error)
	List(ctx context.Context, userID int64) ([]services.Project, error)
	Delete(ctx context.Context, projectID, userID int64) error
}

func (s *Server) registerProjectRoutes(dynamic *http.ServeMux) {
	dynamic.Handle(route(http.MethodGet, paths.Projects), s.requireVerifiedAuth(http.HandlerFunc(s.projectsIndex)))
	dynamic.Handle(route(http.MethodPost, paths.Projects), s.requireVerifiedAuth(http.HandlerFunc(s.createProject)))
	s.postOnly(dynamic, paths.ProjectsDelete, s.requireVerifiedAuth(http.HandlerFunc(s.deleteProject)))
}

// projectsIndex lists the current user's projects and renders the create form.
func (s *Server) projectsIndex(w http.ResponseWriter, r *http.Request) {
	data := s.newTemplateData(w, r, "Projects")
	if !s.populateProjects(w, r, &data) {
		return
	}
	s.render(w, templateProjects, data)
}

// createProject validates the submitted name and stores a new project.
func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if _, err := s.projects.Create(r.Context(), user.ID, r.FormValue("name")); err != nil {
		var fieldError string
		switch {
		case errors.Is(err, services.ErrProjectNameRequired):
			fieldError = "Enter a project name."
		case errors.Is(err, services.ErrProjectNameTooLong):
			fieldError = "Use a shorter project name."
		default:
			s.loggerForRequest(r).Error("create project", "user_id", user.ID, "err", err)
			s.internalServerError(w, r)
			return
		}

		data := s.newTemplateData(w, r, "Projects")
		data.Error = "Check your details and try again."
		data.FieldErrors = map[string]string{"name": fieldError}
		if !s.populateProjects(w, r, &data) {
			return
		}
		s.renderStatus(w, http.StatusUnprocessableEntity, templateProjects, data)
		return
	}

	s.loggerForRequest(r).Info("project created", "user_id", user.ID)
	s.setFlash(w, r, flashSuccess("Project created."))
	http.Redirect(w, r, paths.Projects, http.StatusSeeOther)
}

// deleteProject removes a project the current user owns.
func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	projectID, err := projectIDFromForm(r.FormValue("project_id"))
	if err != nil {
		s.setFlash(w, r, flashError("Select a valid project to delete."))
		http.Redirect(w, r, paths.Projects, http.StatusSeeOther)
		return
	}

	if err := s.projects.Delete(r.Context(), projectID, user.ID); err != nil {
		if errors.Is(err, services.ErrProjectNotFound) {
			s.setFlash(w, r, flashError("That project is no longer available."))
			http.Redirect(w, r, paths.Projects, http.StatusSeeOther)
			return
		}
		s.loggerForRequest(r).Error("delete project", "user_id", user.ID, "project_id", projectID, "err", err)
		s.internalServerError(w, r)
		return
	}

	s.loggerForRequest(r).Info("project deleted", "user_id", user.ID, "project_id", projectID)
	s.setFlash(w, r, flashSuccess("Project deleted."))
	http.Redirect(w, r, paths.Projects, http.StatusSeeOther)
}

// populateProjects loads the current user's projects into data. It returns false
// (after writing a response) when the request could not be served.
func (s *Server) populateProjects(w http.ResponseWriter, r *http.Request, data *templateData) bool {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return false
	}

	projects, err := s.projects.List(r.Context(), user.ID)
	if err != nil {
		s.loggerForRequest(r).Error("list projects", "user_id", user.ID, "err", err)
		s.internalServerError(w, r)
		return false
	}

	data.Projects = projects
	return true
}

func projectIDFromForm(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid project id")
	}
	return id, nil
}
