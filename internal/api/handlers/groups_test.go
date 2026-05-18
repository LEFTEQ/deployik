package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

func createHandlerGroupProject(t *testing.T, database *db.DB, userID, groupID, name string) *db.Project {
	t.Helper()
	project := &db.Project{
		Name:            name,
		GithubRepo:      name + "-repo",
		GithubOwner:     "owner",
		Branch:          "main",
		UserID:          userID,
		OrganizationID:  groupID,
		Framework:       "nextjs",
		PackageManager:  "auto",
		OutputDirectory: ".next",
		BuildCommand:    "bun run build",
		InstallCommand:  "bun install --frozen-lockfile",
		NodeVersion:     "22",
		Status:          "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject(%s): %v", name, err)
	}
	return project
}

func groupJSONReq(t *testing.T, method, path string, body map[string]any, user *db.User) *http.Request {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	return withClaims(req, user.ID, user.Role)
}

func TestGroupCreateMovesSelectedProjects(t *testing.T) {
	database, _, user := setupProjectTestDB(t)
	defaultGroup, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	project := createHandlerGroupProject(t, database, user.ID, defaultGroup.ID, "web")
	handler := &GroupHandler{DB: database}

	req := groupJSONReq(t, http.MethodPost, "/api/groups", map[string]any{
		"name":        "Client Apps",
		"project_ids": []string{project.ID},
	}, user)
	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var group db.Group
	if err := json.Unmarshal(rec.Body.Bytes(), &group); err != nil {
		t.Fatalf("decode group: %v", err)
	}
	moved, err := database.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if moved.OrganizationID != group.ID {
		t.Fatalf("project organization_id = %q, want %q", moved.OrganizationID, group.ID)
	}
}

func TestGroupMemberCannotManageButAdminCan(t *testing.T) {
	database, _, owner := setupProjectTestDB(t)
	member := &db.User{ID: db.NewID(), GithubID: 123, Username: "member", Role: "user"}
	if err := database.UpsertUser(member); err != nil {
		t.Fatalf("UpsertUser(member): %v", err)
	}
	admin := &db.User{ID: db.NewID(), GithubID: 124, Username: "admin", Role: "admin"}
	if err := database.UpsertUser(admin); err != nil {
		t.Fatalf("UpsertUser(admin): %v", err)
	}
	group, err := database.CreateGroup(&db.GroupCreate{Name: "Team", OwnerID: owner.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := database.AddOrganizationMember(group.ID, member.ID, db.OrganizationRoleMember); err != nil {
		t.Fatalf("AddOrganizationMember(member): %v", err)
	}
	handler := &GroupHandler{DB: database}

	memberReq := routeRequest(
		groupJSONReq(t, http.MethodPatch, "/api/groups/"+group.ID, map[string]any{"name": "Blocked"}, member),
		map[string]string{"id": group.ID},
	)
	memberRec := httptest.NewRecorder()
	handler.Update(memberRec, memberReq)
	if memberRec.Code != http.StatusForbidden {
		t.Fatalf("member status = %d, want 403; body = %s", memberRec.Code, memberRec.Body.String())
	}

	adminReq := routeRequest(
		groupJSONReq(t, http.MethodPatch, "/api/groups/"+group.ID, map[string]any{"name": "Admin Renamed"}, admin),
		map[string]string{"id": group.ID},
	)
	adminRec := httptest.NewRecorder()
	handler.Update(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("admin status = %d, want 200; body = %s", adminRec.Code, adminRec.Body.String())
	}
	stored, err := database.GetGroup(group.ID)
	if err != nil {
		t.Fatalf("GetGroup: %v", err)
	}
	if stored.Name != "Admin Renamed" {
		t.Fatalf("group name = %q, want Admin Renamed", stored.Name)
	}
}

func TestGroupDeleteMovesProjectsToPersonalDefault(t *testing.T) {
	database, _, user := setupProjectTestDB(t)
	defaultGroup, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	group, err := database.CreateGroup(&db.GroupCreate{Name: "Temporary", OwnerID: user.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	project := createHandlerGroupProject(t, database, user.ID, group.ID, "temporary-web")
	handler := &GroupHandler{DB: database}

	req := routeRequest(
		withClaims(httptest.NewRequest(http.MethodDelete, "/api/groups/"+group.ID, nil), user.ID, user.Role),
		map[string]string{"id": group.ID},
	)
	rec := httptest.NewRecorder()
	handler.Delete(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	moved, err := database.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if moved.OrganizationID != defaultGroup.ID {
		t.Fatalf("project organization_id = %q, want default %q", moved.OrganizationID, defaultGroup.ID)
	}
}

func TestGroupDeleteByAdminMovesProjectsToGroupOwnerPersonalDefault(t *testing.T) {
	database, _, owner := setupProjectTestDB(t)
	ownerDefaultGroup, err := database.EnsurePersonalOrganization(owner)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization(owner): %v", err)
	}
	admin := &db.User{ID: db.NewID(), GithubID: 400, Username: "admin", Role: "admin"}
	if err := database.UpsertUser(admin); err != nil {
		t.Fatalf("UpsertUser(admin): %v", err)
	}
	adminDefaultGroup, err := database.EnsurePersonalOrganization(admin)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization(admin): %v", err)
	}
	group, err := database.CreateGroup(&db.GroupCreate{Name: "Temporary", OwnerID: owner.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	project := createHandlerGroupProject(t, database, owner.ID, group.ID, "temporary-admin-delete")
	handler := &GroupHandler{DB: database}

	req := routeRequest(
		withClaims(httptest.NewRequest(http.MethodDelete, "/api/groups/"+group.ID, nil), admin.ID, admin.Role),
		map[string]string{"id": group.ID},
	)
	rec := httptest.NewRecorder()
	handler.Delete(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	moved, err := database.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if moved.OrganizationID == adminDefaultGroup.ID {
		t.Fatalf("project was moved to admin default group %q", adminDefaultGroup.ID)
	}
	if moved.OrganizationID != ownerDefaultGroup.ID {
		t.Fatalf("project organization_id = %q, want owner default %q", moved.OrganizationID, ownerDefaultGroup.ID)
	}
}

func TestGroupInviteAcceptDeclineFlow(t *testing.T) {
	database, _, owner := setupProjectTestDB(t)
	invitee := &db.User{ID: db.NewID(), GithubID: 200, Username: "teammate", Role: "user"}
	if err := database.UpsertUser(invitee); err != nil {
		t.Fatalf("UpsertUser(invitee): %v", err)
	}
	group, err := database.CreateGroup(&db.GroupCreate{Name: "Team", OwnerID: owner.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	handler := &GroupHandler{DB: database}

	createReq := routeRequest(
		groupJSONReq(t, http.MethodPost, "/api/groups/"+group.ID+"/invites", map[string]any{
			"github_username": "Teammate",
			"role":            db.OrganizationRoleMember,
		}, owner),
		map[string]string{"id": group.ID},
	)
	createRec := httptest.NewRecorder()
	handler.CreateInvite(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create invite status = %d, body = %s", createRec.Code, createRec.Body.String())
	}
	var invite db.GroupInvite
	if err := json.Unmarshal(createRec.Body.Bytes(), &invite); err != nil {
		t.Fatalf("decode invite: %v", err)
	}

	listReq := withClaims(httptest.NewRequest(http.MethodGet, "/api/me/group-invites", nil), invitee.ID, invitee.Role)
	listRec := httptest.NewRecorder()
	handler.ListMyInvites(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list invites status = %d, body = %s", listRec.Code, listRec.Body.String())
	}
	var pending []db.GroupInvite
	if err := json.Unmarshal(listRec.Body.Bytes(), &pending); err != nil {
		t.Fatalf("decode pending: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != invite.ID {
		t.Fatalf("pending invites = %#v, want invite %s", pending, invite.ID)
	}

	acceptReq := routeRequest(
		withClaims(httptest.NewRequest(http.MethodPost, "/api/me/group-invites/"+invite.ID+"/accept", nil), invitee.ID, invitee.Role),
		map[string]string{"iid": invite.ID},
	)
	acceptRec := httptest.NewRecorder()
	handler.AcceptInvite(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusNoContent {
		t.Fatalf("accept status = %d, body = %s", acceptRec.Code, acceptRec.Body.String())
	}
	memberGroup, err := database.GetOrganizationForUser(group.ID, invitee.ID)
	if err != nil {
		t.Fatalf("GetOrganizationForUser: %v", err)
	}
	if memberGroup == nil || memberGroup.MembershipRole != db.OrganizationRoleMember {
		t.Fatalf("invitee membership = %#v, want member", memberGroup)
	}
}
