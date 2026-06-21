package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/inkyvoxel/go-spark/internal/sqlitetest"
)

func newProjectTestService(t *testing.T) (*ProjectService, *sql.DB) {
	t.Helper()
	db := sqlitetest.New(t)
	return NewProjectService(db), db
}

var testUserSeq atomic.Int64

// createTestUser inserts a minimal valid user and returns its ID, so project
// rows satisfy the projects.user_id foreign key the real schema enforces.
func createTestUser(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	n := testUserSeq.Add(1)
	var id int64
	err := db.QueryRow(
		`INSERT INTO users (email, password_hash, webauthn_user_handle)
		 VALUES (?, ?, ?) RETURNING id`,
		fmt.Sprintf("user%d@example.com", n), "hash", []byte(fmt.Sprintf("handle-%d", n)),
	).Scan(&id)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return id
}

func TestProjectServiceCreateTrimsAndStores(t *testing.T) {
	svc, db := newProjectTestService(t)
	ctx := context.Background()
	userID := createTestUser(t, db)

	project, err := svc.Create(ctx, userID, "  My Project  ")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if project.Name != "My Project" || project.UserID != userID {
		t.Fatalf("project = %+v, want name trimmed and user %d", project, userID)
	}
	if project.ID == 0 || project.CreatedAt.IsZero() {
		t.Fatalf("project missing persisted fields: %+v", project)
	}

	list, err := svc.List(ctx, userID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].Name != "My Project" {
		t.Fatalf("list = %+v, want one stored project named %q", list, "My Project")
	}
}

func TestProjectServiceCreateRejectsEmptyName(t *testing.T) {
	svc, db := newProjectTestService(t)
	ctx := context.Background()
	userID := createTestUser(t, db)

	if _, err := svc.Create(ctx, userID, "   "); !errors.Is(err, ErrProjectNameRequired) {
		t.Fatalf("Create() error = %v, want ErrProjectNameRequired", err)
	}
	if list, _ := svc.List(ctx, userID); len(list) != 0 {
		t.Fatalf("rejected name was stored: %+v", list)
	}
}

func TestProjectServiceCreateRejectsTooLongName(t *testing.T) {
	// Validation rejects before any insert, so no user row is needed.
	svc, _ := newProjectTestService(t)

	tooLong := strings.Repeat("a", ProjectNameMaxLength+1)
	if _, err := svc.Create(context.Background(), 1, tooLong); !errors.Is(err, ErrProjectNameTooLong) {
		t.Fatalf("Create() error = %v, want ErrProjectNameTooLong", err)
	}
}

func TestProjectServiceListIsUserScopedAndOrdered(t *testing.T) {
	svc, db := newProjectTestService(t)
	ctx := context.Background()
	owner := createTestUser(t, db)
	other := createTestUser(t, db)

	if _, err := svc.Create(ctx, owner, "Alpha"); err != nil {
		t.Fatalf("Create(Alpha) error = %v", err)
	}
	if _, err := svc.Create(ctx, owner, "Beta"); err != nil {
		t.Fatalf("Create(Beta) error = %v", err)
	}
	// Another user's project must not leak into the owner's list.
	if _, err := svc.Create(ctx, other, "Other"); err != nil {
		t.Fatalf("Create(Other) error = %v", err)
	}

	list, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list length = %d, want 2 (user-scoped)", len(list))
	}
	// Newest first; equal timestamps break ties by id DESC, so Beta leads.
	if list[0].Name != "Beta" {
		t.Fatalf("first project = %q, want newest %q", list[0].Name, "Beta")
	}
}

func TestProjectServiceDeleteScopesToUserAndReportsMiss(t *testing.T) {
	svc, db := newProjectTestService(t)
	ctx := context.Background()
	owner := createTestUser(t, db)
	other := createTestUser(t, db)

	project, err := svc.Create(ctx, owner, "Alpha")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Another user cannot delete this user's project.
	if err := svc.Delete(ctx, project.ID, other); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("cross-user Delete() error = %v, want ErrProjectNotFound", err)
	}

	if err := svc.Delete(ctx, project.ID, owner); err != nil {
		t.Fatalf("owner Delete() error = %v", err)
	}
	if list, _ := svc.List(ctx, owner); len(list) != 0 {
		t.Fatalf("project not deleted: %+v", list)
	}
}
