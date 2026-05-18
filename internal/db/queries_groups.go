package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrDefaultGroupCannotBeDeleted = errors.New("default group cannot be deleted")
	ErrLastGroupOwner              = errors.New("group must have at least one owner")
	ErrGroupInviteNotFound         = errors.New("group invite not found")
	ErrProjectNotMovable           = errors.New("project not found or not movable")
)

func (db *DB) ListGroupsForUser(userID string) ([]Group, error) {
	rows, err := db.Query(
		`SELECT o.id, o.name, o.slug, o.is_personal, COALESCE(o.personal_owner_user_id, ''),
		        om.role, COUNT(p.id), o.display_order, o.created_at, o.updated_at
		 FROM organizations o
		 JOIN organization_memberships om ON om.organization_id = o.id
		 LEFT JOIN projects p ON p.organization_id = o.id AND p.status != 'deleted'
		 WHERE om.user_id = ?
		 GROUP BY o.id, o.name, o.slug, o.is_personal, o.personal_owner_user_id,
		          om.role, o.display_order, o.created_at, o.updated_at
		 ORDER BY o.is_personal ASC, o.display_order ASC, lower(o.name) ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list groups for user: %w", err)
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var group Group
		if err := rows.Scan(
			&group.ID,
			&group.Name,
			&group.Slug,
			&group.IsDefault,
			&group.PersonalOwnerUserID,
			&group.MembershipRole,
			&group.ProjectCount,
			&group.DisplayOrder,
			&group.CreatedAt,
			&group.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan group: %w", err)
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (db *DB) GetGroupForUser(groupID, userID string) (*Group, error) {
	group := &Group{}
	err := db.QueryRow(
		`SELECT o.id, o.name, o.slug, o.is_personal, COALESCE(o.personal_owner_user_id, ''),
		        om.role,
		        (SELECT COUNT(*) FROM projects p WHERE p.organization_id = o.id AND p.status != 'deleted'),
		        o.display_order, o.created_at, o.updated_at
		 FROM organizations o
		 JOIN organization_memberships om ON om.organization_id = o.id
		 WHERE o.id = ? AND om.user_id = ?`,
		groupID, userID,
	).Scan(
		&group.ID,
		&group.Name,
		&group.Slug,
		&group.IsDefault,
		&group.PersonalOwnerUserID,
		&group.MembershipRole,
		&group.ProjectCount,
		&group.DisplayOrder,
		&group.CreatedAt,
		&group.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get group for user: %w", err)
	}
	return group, nil
}

func (db *DB) CreateGroup(input *GroupCreate) (*Group, error) {
	if input == nil {
		return nil, fmt.Errorf("create group: input is nil")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("create group: name is required")
	}
	ownerID := strings.TrimSpace(input.OwnerID)
	if ownerID == "" {
		return nil, fmt.Errorf("create group: owner_id is required")
	}
	slug, err := db.reserveUniqueOrganizationSlug(slugifyOrganizationName(name))
	if err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}

	var displayOrder int
	if err := db.QueryRow(`SELECT COALESCE(MAX(display_order), 0) + 1 FROM organizations`).Scan(&displayOrder); err != nil {
		return nil, fmt.Errorf("create group: next display order: %w", err)
	}

	groupID := NewID()
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("create group: begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO organizations (id, name, slug, is_personal, display_order)
		 VALUES (?, ?, ?, 0, ?)`,
		groupID, name, slug, displayOrder,
	); err != nil {
		return nil, fmt.Errorf("create group: insert organization: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO organization_memberships (organization_id, user_id, role)
		 VALUES (?, ?, ?)`,
		groupID, ownerID, OrganizationRoleOwner,
	); err != nil {
		return nil, fmt.Errorf("create group: add owner: %w", err)
	}
	for _, projectID := range uniqueNonEmpty(input.ProjectIDs) {
		if err := moveProjectToGroupTx(tx, groupID, projectID); err != nil {
			return nil, fmt.Errorf("create group: move project: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create group: commit: %w", err)
	}

	group, err := db.GetGroupForUser(groupID, ownerID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, fmt.Errorf("create group: created group not found")
	}
	return group, nil
}

func (db *DB) UpdateGroupName(groupID, name string) (*Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("update group name: name is required")
	}
	_, err := db.Exec(
		`UPDATE organizations
		 SET name = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		name, groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("update group name: %w", err)
	}
	return db.GetGroup(groupID)
}

func (db *DB) GetGroup(groupID string) (*Group, error) {
	group := &Group{}
	err := db.QueryRow(
		`SELECT o.id, o.name, o.slug, o.is_personal, COALESCE(o.personal_owner_user_id, ''),
		        COALESCE((
		          SELECT COUNT(*) FROM projects p WHERE p.organization_id = o.id AND p.status != 'deleted'
		        ), 0),
		        o.display_order, o.created_at, o.updated_at
		 FROM organizations o
		 WHERE o.id = ?`,
		groupID,
	).Scan(
		&group.ID,
		&group.Name,
		&group.Slug,
		&group.IsDefault,
		&group.PersonalOwnerUserID,
		&group.ProjectCount,
		&group.DisplayOrder,
		&group.CreatedAt,
		&group.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get group: %w", err)
	}
	return group, nil
}

func (db *DB) MoveProjectsToGroup(groupID string, projectIDs []string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("move projects to group: begin: %w", err)
	}
	defer tx.Rollback()
	for _, projectID := range uniqueNonEmpty(projectIDs) {
		if err := moveProjectToGroupTx(tx, groupID, projectID); err != nil {
			return fmt.Errorf("move projects to group: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("move projects to group: commit: %w", err)
	}
	return nil
}

func moveProjectToGroupTx(tx *sql.Tx, groupID, projectID string) error {
	res, err := tx.Exec(
		`UPDATE projects
		 SET organization_id = ?, updated_at = datetime('now')
		 WHERE id = ? AND status != 'deleted'`,
		groupID, projectID,
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check moved project count: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: %s", ErrProjectNotMovable, projectID)
	}
	return nil
}

func (db *DB) DeleteGroupMovingProjects(groupID, defaultGroupID string) error {
	group, err := db.GetGroup(groupID)
	if err != nil {
		return err
	}
	if group == nil {
		return nil
	}
	if group.IsDefault {
		return ErrDefaultGroupCannotBeDeleted
	}
	defaultGroup, err := db.GetGroup(defaultGroupID)
	if err != nil {
		return err
	}
	if defaultGroup == nil || !defaultGroup.IsDefault {
		return fmt.Errorf("delete group: default group not found")
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("delete group: begin: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`UPDATE projects
		 SET organization_id = ?, updated_at = datetime('now')
		 WHERE organization_id = ? AND status != 'deleted'`,
		defaultGroupID, groupID,
	); err != nil {
		return fmt.Errorf("delete group: move projects: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM organizations WHERE id = ?`, groupID); err != nil {
		return fmt.Errorf("delete group: delete organization: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete group: commit: %w", err)
	}
	return nil
}

func (db *DB) ResolveDefaultGroupForGroup(groupID string) (*Organization, error) {
	owner, err := db.GetGroupPrimaryOwner(groupID)
	if err != nil {
		return nil, err
	}
	if owner == nil {
		return nil, fmt.Errorf("resolve default group: group owner not found")
	}
	defaultGroup, err := db.EnsurePersonalOrganization(owner)
	if err != nil {
		return nil, fmt.Errorf("resolve default group: %w", err)
	}
	return defaultGroup, nil
}

func (db *DB) GetGroupPrimaryOwner(groupID string) (*User, error) {
	user := &User{}
	err := db.QueryRow(
		`SELECT u.id, u.github_id, u.username, u.avatar_url, u.github_token, u.role, u.created_at
		 FROM organization_memberships om
		 JOIN users u ON u.id = om.user_id
		 WHERE om.organization_id = ? AND om.role = 'owner'
		 ORDER BY om.created_at ASC, u.id ASC
		 LIMIT 1`,
		groupID,
	).Scan(&user.ID, &user.GithubID, &user.Username, &user.AvatarURL, &user.GithubToken, &user.Role, &user.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get group primary owner: %w", err)
	}
	return user, nil
}

func (db *DB) ListGroupMembers(groupID string) ([]GroupMember, error) {
	rows, err := db.Query(
		`SELECT om.organization_id, u.id, u.username, u.avatar_url, om.role, om.created_at
		 FROM organization_memberships om
		 JOIN users u ON u.id = om.user_id
		 WHERE om.organization_id = ?
		 ORDER BY CASE om.role WHEN 'owner' THEN 0 ELSE 1 END, lower(u.username) ASC`,
		groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("list group members: %w", err)
	}
	defer rows.Close()

	var members []GroupMember
	for rows.Next() {
		var member GroupMember
		if err := rows.Scan(
			&member.GroupID,
			&member.UserID,
			&member.Username,
			&member.AvatarURL,
			&member.Role,
			&member.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan group member: %w", err)
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func (db *DB) UpdateGroupMemberRole(groupID, userID, role string) error {
	role = normalizeOrganizationRole(role)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("update group member role: begin: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`UPDATE organization_memberships
		 SET role = ?
		 WHERE organization_id = ? AND user_id = ?
		   AND (
		     role != 'owner'
		     OR ? = 'owner'
		     OR EXISTS (
		       SELECT 1
		       FROM organization_memberships
		       WHERE organization_id = ? AND user_id != ? AND role = 'owner'
		     )
		   )`,
		role, groupID, userID, role, groupID, userID,
	)
	if err != nil {
		return fmt.Errorf("update group member role: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update group member role: check rows affected: %w", err)
	}
	if affected == 0 {
		currentRole, err := getGroupMemberRoleTx(tx, groupID, userID)
		if err != nil {
			return err
		}
		if currentRole == OrganizationRoleOwner && role != OrganizationRoleOwner {
			return ErrLastGroupOwner
		}
		return nil
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("update group member role: commit: %w", err)
	}
	return nil
}

func (db *DB) RemoveGroupMember(groupID, userID string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("remove group member: begin: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`DELETE FROM organization_memberships
		 WHERE organization_id = ? AND user_id = ?
		   AND (
		     role != 'owner'
		     OR EXISTS (
		       SELECT 1
		       FROM organization_memberships
		       WHERE organization_id = ? AND user_id != ? AND role = 'owner'
		     )
		   )`,
		groupID, userID, groupID, userID,
	)
	if err != nil {
		return fmt.Errorf("remove group member: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove group member: check rows affected: %w", err)
	}
	if affected == 0 {
		currentRole, err := getGroupMemberRoleTx(tx, groupID, userID)
		if err != nil {
			return err
		}
		if currentRole == OrganizationRoleOwner {
			return ErrLastGroupOwner
		}
		return nil
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("remove group member: commit: %w", err)
	}
	return nil
}

type queryRower interface {
	QueryRow(query string, args ...any) *sql.Row
}

func getGroupMemberRoleTx(q queryRower, groupID, userID string) (string, error) {
	var role string
	err := q.QueryRow(
		`SELECT role FROM organization_memberships WHERE organization_id = ? AND user_id = ?`,
		groupID, userID,
	).Scan(&role)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get group member role: %w", err)
	}
	return role, nil
}

func (db *DB) CreateGroupInvite(input *GroupInviteCreate) (*GroupInvite, error) {
	if input == nil {
		return nil, fmt.Errorf("create group invite: input is nil")
	}
	username := normalizeGithubUsername(input.GithubUsername)
	if username == "" {
		return nil, fmt.Errorf("create group invite: github_username is required")
	}
	role := normalizeOrganizationRole(input.Role)
	inviteID := NewID()
	_, err := db.Exec(
		`INSERT INTO group_invitations (id, organization_id, github_username, role, invited_by_user_id)
		 VALUES (?, ?, ?, ?, ?)`,
		inviteID, input.GroupID, username, role, input.InvitedByUserID,
	)
	if err != nil {
		return nil, fmt.Errorf("create group invite: %w", err)
	}
	return db.GetGroupInvite(inviteID)
}

func (db *DB) GetGroupInvite(inviteID string) (*GroupInvite, error) {
	invite := &GroupInvite{}
	err := db.QueryRow(groupInviteSelectSQL()+` WHERE gi.id = ?`, inviteID).Scan(groupInviteScanTargets(invite)...)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get group invite: %w", err)
	}
	return invite, nil
}

func (db *DB) ListGroupInvites(groupID string) ([]GroupInvite, error) {
	rows, err := db.Query(
		groupInviteSelectSQL()+` WHERE gi.organization_id = ? AND gi.status = 'pending' ORDER BY lower(gi.github_username) ASC`,
		groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("list group invites: %w", err)
	}
	defer rows.Close()
	return scanGroupInvites(rows)
}

func (db *DB) ListPendingGroupInvitesForGithubUsername(username string) ([]GroupInvite, error) {
	rows, err := db.Query(
		groupInviteSelectSQL()+` WHERE gi.github_username = ? AND gi.status = 'pending' ORDER BY gi.created_at DESC`,
		normalizeGithubUsername(username),
	)
	if err != nil {
		return nil, fmt.Errorf("list pending group invites: %w", err)
	}
	defer rows.Close()
	return scanGroupInvites(rows)
}

func (db *DB) AcceptGroupInvite(inviteID, userID, githubUsername string) error {
	return db.resolveGroupInvite(inviteID, userID, githubUsername, "accepted")
}

func (db *DB) DeclineGroupInvite(inviteID, githubUsername string) error {
	return db.resolveGroupInvite(inviteID, "", githubUsername, "declined")
}

func (db *DB) CancelGroupInvite(groupID, inviteID string) error {
	_, err := db.Exec(
		`UPDATE group_invitations
		 SET status = 'canceled', responded_at = datetime('now'), updated_at = datetime('now')
		 WHERE id = ? AND organization_id = ? AND status = 'pending'`,
		inviteID, groupID,
	)
	if err != nil {
		return fmt.Errorf("cancel group invite: %w", err)
	}
	return nil
}

func (db *DB) resolveGroupInvite(inviteID, userID, githubUsername, status string) error {
	username := normalizeGithubUsername(githubUsername)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("resolve group invite: begin: %w", err)
	}
	defer tx.Rollback()

	var groupID, role string
	err = tx.QueryRow(
		`SELECT organization_id, role
		 FROM group_invitations
		 WHERE id = ? AND github_username = ? AND status = 'pending'`,
		inviteID, username,
	).Scan(&groupID, &role)
	if err == sql.ErrNoRows {
		return ErrGroupInviteNotFound
	}
	if err != nil {
		return fmt.Errorf("resolve group invite: load invite: %w", err)
	}

	if status == "accepted" {
		if strings.TrimSpace(userID) == "" {
			return fmt.Errorf("resolve group invite: user_id is required")
		}
		if err := upsertOrganizationMember(tx, groupID, userID, role); err != nil {
			return fmt.Errorf("resolve group invite: add member: %w", err)
		}
	}

	if _, err := tx.Exec(
		`UPDATE group_invitations
		 SET status = ?, responded_at = datetime('now'), updated_at = datetime('now')
		 WHERE id = ?`,
		status, inviteID,
	); err != nil {
		return fmt.Errorf("resolve group invite: update invite: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("resolve group invite: commit: %w", err)
	}
	return nil
}

func groupInviteSelectSQL() string {
	return `SELECT gi.id, gi.organization_id, o.name, gi.github_username, gi.role,
		       gi.invited_by_user_id, COALESCE(u.username, ''), gi.status,
		       gi.responded_at, gi.created_at, gi.updated_at
		FROM group_invitations gi
		JOIN organizations o ON o.id = gi.organization_id
		LEFT JOIN users u ON u.id = gi.invited_by_user_id`
}

func groupInviteScanTargets(invite *GroupInvite) []any {
	return []any{
		&invite.ID,
		&invite.GroupID,
		&invite.GroupName,
		&invite.GithubUsername,
		&invite.Role,
		&invite.InvitedByUserID,
		&invite.InvitedByUsername,
		&invite.Status,
		&invite.RespondedAt,
		&invite.CreatedAt,
		&invite.UpdatedAt,
	}
}

func scanGroupInvites(rows *sql.Rows) ([]GroupInvite, error) {
	var invites []GroupInvite
	for rows.Next() {
		var invite GroupInvite
		if err := rows.Scan(groupInviteScanTargets(&invite)...); err != nil {
			return nil, fmt.Errorf("scan group invite: %w", err)
		}
		invites = append(invites, invite)
	}
	return invites, rows.Err()
}

func normalizeGithubUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
