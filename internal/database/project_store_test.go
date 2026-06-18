package database

import (
	"context"
	"database/sql"
	"testing"
)

const projectStoreTestSchema = `
CREATE TABLE projects (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

func newProjectStoreTestDB(t *testing.T) *sql.DB {
	t.Helper()

	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if _, err := conn.Exec(projectStoreTestSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return conn
}

func TestProjectStoreCreateListDelete(t *testing.T) {
	store := NewProjectStore(newProjectStoreTestDB(t))
	ctx := context.Background()

	alpha, err := store.CreateProject(ctx, 1, "Alpha")
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if alpha.ID == 0 || alpha.Name != "Alpha" || alpha.UserID != 1 {
		t.Fatalf("created project = %+v", alpha)
	}
	if alpha.CreatedAt.IsZero() {
		t.Fatal("created project has zero CreatedAt")
	}

	if _, err := store.CreateProject(ctx, 1, "Beta"); err != nil {
		t.Fatalf("CreateProject(Beta) error = %v", err)
	}
	// A project owned by a different user must not leak into user 1's list.
	if _, err := store.CreateProject(ctx, 2, "Other"); err != nil {
		t.Fatalf("CreateProject(Other) error = %v", err)
	}

	list, err := store.ListProjectsByUserID(ctx, 1)
	if err != nil {
		t.Fatalf("ListProjectsByUserID() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list length = %d, want 2 (user-scoped)", len(list))
	}
	// Ordered newest first; equal timestamps break ties by id DESC, so Beta (id 2) leads.
	if list[0].Name != "Beta" {
		t.Fatalf("first project = %q, want newest %q", list[0].Name, "Beta")
	}

	// Another user cannot delete this user's project.
	deleted, err := store.DeleteProject(ctx, alpha.ID, 2)
	if err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}
	if deleted {
		t.Fatal("cross-user delete reported success; ownership not enforced")
	}

	deleted, err = store.DeleteProject(ctx, alpha.ID, 1)
	if err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}
	if !deleted {
		t.Fatal("owner delete reported no rows affected")
	}

	list, err = store.ListProjectsByUserID(ctx, 1)
	if err != nil {
		t.Fatalf("ListProjectsByUserID() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list length after delete = %d, want 1", len(list))
	}
}
