package db

import (
	"errors"
	"testing"
)

func createGroupTestUser(t *testing.T, database *DB, username, role string, githubID int64) *User {
	t.Helper()
	user := &User{ID: NewID(), GithubID: githubID, Username: username, Role: role}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser(%s): %v", username, err)
	}
	return user
}

func createGroupTestProject(t *testing.T, database *DB, userID, groupID, name string) *Project {
	t.Helper()
	project := &Project{
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

func TestCreateGroupMovesSelectedProjects(t *testing.T) {
	database := newTestDB(t)
	user := createGroupTestUser(t, database, "owner", "user", 1)
	defaultGroup, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	project := createGroupTestProject(t, database, user.ID, defaultGroup.ID, "web")

	group, err := database.CreateGroup(&GroupCreate{
		Name:       "Client Apps",
		OwnerID:    user.ID,
		ProjectIDs: []string{project.ID},
	})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	if group.ID == "" {
		t.Fatal("expected group id to be generated")
	}
	if group.Name != "Client Apps" {
		t.Fatalf("group name = %q, want Client Apps", group.Name)
	}
	if group.MembershipRole != OrganizationRoleOwner {
		t.Fatalf("membership role = %q, want owner", group.MembershipRole)
	}
	if group.IsDefault {
		t.Fatal("custom group should not be default")
	}

	groups, err := database.ListGroupsForUser(user.ID)
	if err != nil {
		t.Fatalf("ListGroupsForUser: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	if groups[0].IsDefault {
		t.Fatalf("default group should not sort before custom group")
	}

	moved, err := database.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if moved.OrganizationID != group.ID {
		t.Fatalf("project organization_id = %q, want %q", moved.OrganizationID, group.ID)
	}
}

func TestCreateGroupRejectsMissingProjectIDsAtomically(t *testing.T) {
	database := newTestDB(t)
	user := createGroupTestUser(t, database, "owner", "user", 1)
	defaultGroup, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	project := createGroupTestProject(t, database, user.ID, defaultGroup.ID, "web")

	if _, err := database.CreateGroup(&GroupCreate{
		Name:       "Client Apps",
		OwnerID:    user.ID,
		ProjectIDs: []string{project.ID, "missing-project"},
	}); err == nil {
		t.Fatal("CreateGroup error = nil, want missing project error")
	}

	moved, err := database.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if moved.OrganizationID != defaultGroup.ID {
		t.Fatalf("project organization_id = %q, want original %q", moved.OrganizationID, defaultGroup.ID)
	}

	groups, err := database.ListGroupsForUser(user.ID)
	if err != nil {
		t.Fatalf("ListGroupsForUser: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("got %d groups after failed create, want only personal group", len(groups))
	}
}

func TestMoveProjectsToGroupRejectsMissingProjectIDsAtomically(t *testing.T) {
	database := newTestDB(t)
	user := createGroupTestUser(t, database, "owner", "user", 1)
	sourceGroup, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	targetGroup, err := database.CreateGroup(&GroupCreate{Name: "Client Apps", OwnerID: user.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	project := createGroupTestProject(t, database, user.ID, sourceGroup.ID, "web")

	if err := database.MoveProjectsToGroup(targetGroup.ID, []string{project.ID, "missing-project"}); err == nil {
		t.Fatal("MoveProjectsToGroup error = nil, want missing project error")
	}

	moved, err := database.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if moved.OrganizationID != sourceGroup.ID {
		t.Fatalf("project organization_id = %q, want original %q", moved.OrganizationID, sourceGroup.ID)
	}
}

func TestDeleteGroupMovesProjectsToDefault(t *testing.T) {
	database := newTestDB(t)
	user := createGroupTestUser(t, database, "owner", "user", 1)
	defaultGroup, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	group, err := database.CreateGroup(&GroupCreate{Name: "Temporary", OwnerID: user.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	project := createGroupTestProject(t, database, user.ID, group.ID, "temporary-web")

	if err := database.DeleteGroupMovingProjects(group.ID, defaultGroup.ID); err != nil {
		t.Fatalf("DeleteGroupMovingProjects: %v", err)
	}

	deleted, err := database.GetOrganization(group.ID)
	if err != nil {
		t.Fatalf("GetOrganization(deleted): %v", err)
	}
	if deleted != nil {
		t.Fatal("expected group row to be deleted")
	}
	moved, err := database.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if moved.OrganizationID != defaultGroup.ID {
		t.Fatalf("project organization_id = %q, want default %q", moved.OrganizationID, defaultGroup.ID)
	}

	if err := database.DeleteGroupMovingProjects(defaultGroup.ID, defaultGroup.ID); !errors.Is(err, ErrDefaultGroupCannotBeDeleted) {
		t.Fatalf("deleting default group error = %v, want ErrDefaultGroupCannotBeDeleted", err)
	}
}

func TestGroupInvitesCanBeAcceptedOrDeclinedByGithubUsername(t *testing.T) {
	database := newTestDB(t)
	owner := createGroupTestUser(t, database, "owner", "user", 1)
	invitee := createGroupTestUser(t, database, "teammate", "user", 2)
	group, err := database.CreateGroup(&GroupCreate{Name: "Team", OwnerID: owner.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	invite, err := database.CreateGroupInvite(&GroupInviteCreate{
		GroupID:         group.ID,
		GithubUsername:  "Teammate",
		Role:            OrganizationRoleMember,
		InvitedByUserID: owner.ID,
	})
	if err != nil {
		t.Fatalf("CreateGroupInvite: %v", err)
	}
	if invite.GithubUsername != "teammate" {
		t.Fatalf("github username = %q, want teammate", invite.GithubUsername)
	}

	pending, err := database.ListPendingGroupInvitesForGithubUsername("TEAMMATE")
	if err != nil {
		t.Fatalf("ListPendingGroupInvitesForGithubUsername: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("got %d pending invites, want 1", len(pending))
	}

	if err := database.AcceptGroupInvite(invite.ID, invitee.ID, invitee.Username); err != nil {
		t.Fatalf("AcceptGroupInvite: %v", err)
	}
	memberGroup, err := database.GetOrganizationForUser(group.ID, invitee.ID)
	if err != nil {
		t.Fatalf("GetOrganizationForUser(invitee): %v", err)
	}
	if memberGroup == nil || memberGroup.MembershipRole != OrganizationRoleMember {
		t.Fatalf("invitee membership = %#v, want member", memberGroup)
	}

	declineInvite, err := database.CreateGroupInvite(&GroupInviteCreate{
		GroupID:         group.ID,
		GithubUsername:  "decliner",
		Role:            OrganizationRoleMember,
		InvitedByUserID: owner.ID,
	})
	if err != nil {
		t.Fatalf("CreateGroupInvite(decline): %v", err)
	}
	if err := database.DeclineGroupInvite(declineInvite.ID, "decliner"); err != nil {
		t.Fatalf("DeclineGroupInvite: %v", err)
	}
	pendingDecliner, err := database.ListPendingGroupInvitesForGithubUsername("decliner")
	if err != nil {
		t.Fatalf("ListPendingGroupInvitesForGithubUsername(decliner): %v", err)
	}
	if len(pendingDecliner) != 0 {
		t.Fatalf("got %d pending invites after decline, want 0", len(pendingDecliner))
	}
}

func TestAcceptGroupInviteDoesNotDemoteExistingOwner(t *testing.T) {
	database := newTestDB(t)
	owner := createGroupTestUser(t, database, "owner", "user", 1)
	group, err := database.CreateGroup(&GroupCreate{Name: "Team", OwnerID: owner.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	invite, err := database.CreateGroupInvite(&GroupInviteCreate{
		GroupID:         group.ID,
		GithubUsername:  owner.Username,
		Role:            OrganizationRoleMember,
		InvitedByUserID: owner.ID,
	})
	if err != nil {
		t.Fatalf("CreateGroupInvite: %v", err)
	}

	if err := database.AcceptGroupInvite(invite.ID, owner.ID, owner.Username); err != nil {
		t.Fatalf("AcceptGroupInvite: %v", err)
	}

	members, err := database.ListGroupMembers(group.ID)
	if err != nil {
		t.Fatalf("ListGroupMembers: %v", err)
	}
	if len(members) != 1 || members[0].UserID != owner.ID || members[0].Role != OrganizationRoleOwner {
		t.Fatalf("members = %#v, want existing owner to stay owner", members)
	}
}

func TestGroupMemberChangesKeepAtLeastOneOwner(t *testing.T) {
	database := newTestDB(t)
	owner := createGroupTestUser(t, database, "owner", "user", 1)
	member := createGroupTestUser(t, database, "member", "user", 2)
	group, err := database.CreateGroup(&GroupCreate{Name: "Team", OwnerID: owner.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := database.AddOrganizationMember(group.ID, member.ID, OrganizationRoleMember); err != nil {
		t.Fatalf("AddOrganizationMember(member): %v", err)
	}

	if err := database.UpdateGroupMemberRole(group.ID, owner.ID, OrganizationRoleMember); !errors.Is(err, ErrLastGroupOwner) {
		t.Fatalf("demote last owner error = %v, want ErrLastGroupOwner", err)
	}
	if err := database.RemoveGroupMember(group.ID, owner.ID); !errors.Is(err, ErrLastGroupOwner) {
		t.Fatalf("remove last owner error = %v, want ErrLastGroupOwner", err)
	}

	if err := database.UpdateGroupMemberRole(group.ID, member.ID, OrganizationRoleOwner); err != nil {
		t.Fatalf("promote member: %v", err)
	}
	if err := database.RemoveGroupMember(group.ID, owner.ID); err != nil {
		t.Fatalf("remove original owner after promotion: %v", err)
	}
	remaining, err := database.ListGroupMembers(group.ID)
	if err != nil {
		t.Fatalf("ListGroupMembers: %v", err)
	}
	if len(remaining) != 1 || remaining[0].UserID != member.ID || remaining[0].Role != OrganizationRoleOwner {
		t.Fatalf("remaining members = %#v, want promoted owner only", remaining)
	}
}
