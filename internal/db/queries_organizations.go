package db

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

const (
	OrganizationRoleOwner  = "owner"
	OrganizationRoleMember = "member"
)

var organizationSlugRegex = regexp.MustCompile(`[^a-z0-9]+`)

func (db *DB) ListOrganizationsForUser(userID string) ([]Organization, error) {
	rows, err := db.Query(
		`SELECT o.id, o.name, o.slug, o.is_personal, COALESCE(o.personal_owner_user_id, ''),
		        om.role, COUNT(p.id), o.created_at, o.updated_at
		 FROM organizations o
		 JOIN organization_memberships om ON om.organization_id = o.id
		 LEFT JOIN projects p ON p.organization_id = o.id AND p.status != 'deleted'
		 WHERE om.user_id = ?
		 GROUP BY o.id, o.name, o.slug, o.is_personal, o.personal_owner_user_id, om.role, o.created_at, o.updated_at
		 ORDER BY o.is_personal ASC, lower(o.name) ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list organizations for user: %w", err)
	}
	defer rows.Close()

	var organizations []Organization
	for rows.Next() {
		var organization Organization
		if err := rows.Scan(
			&organization.ID,
			&organization.Name,
			&organization.Slug,
			&organization.IsPersonal,
			&organization.PersonalOwnerUserID,
			&organization.MembershipRole,
			&organization.ProjectCount,
			&organization.CreatedAt,
			&organization.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan organization: %w", err)
		}
		organizations = append(organizations, organization)
	}

	return organizations, rows.Err()
}

func (db *DB) GetOrganization(id string) (*Organization, error) {
	organization := &Organization{}
	err := db.QueryRow(
		`SELECT id, name, slug, is_personal, COALESCE(personal_owner_user_id, ''), created_at, updated_at
		 FROM organizations
		 WHERE id = ?`,
		id,
	).Scan(
		&organization.ID,
		&organization.Name,
		&organization.Slug,
		&organization.IsPersonal,
		&organization.PersonalOwnerUserID,
		&organization.CreatedAt,
		&organization.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get organization: %w", err)
	}
	return organization, nil
}

func (db *DB) GetOrganizationForUser(id, userID string) (*Organization, error) {
	organization := &Organization{}
	err := db.QueryRow(
		`SELECT o.id, o.name, o.slug, o.is_personal, COALESCE(o.personal_owner_user_id, ''), om.role, o.created_at, o.updated_at
		 FROM organizations o
		 JOIN organization_memberships om ON om.organization_id = o.id
		 WHERE o.id = ? AND om.user_id = ?`,
		id, userID,
	).Scan(
		&organization.ID,
		&organization.Name,
		&organization.Slug,
		&organization.IsPersonal,
		&organization.PersonalOwnerUserID,
		&organization.MembershipRole,
		&organization.CreatedAt,
		&organization.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get organization for user: %w", err)
	}
	return organization, nil
}

func (db *DB) GetPersonalOrganizationForUser(userID string) (*Organization, error) {
	organization := &Organization{}
	err := db.QueryRow(
		`SELECT o.id, o.name, o.slug, o.is_personal, COALESCE(o.personal_owner_user_id, ''), COALESCE(om.role, ''), o.created_at, o.updated_at
		 FROM organizations o
		 LEFT JOIN organization_memberships om ON om.organization_id = o.id AND om.user_id = ?
		 WHERE o.personal_owner_user_id = ?`,
		userID, userID,
	).Scan(
		&organization.ID,
		&organization.Name,
		&organization.Slug,
		&organization.IsPersonal,
		&organization.PersonalOwnerUserID,
		&organization.MembershipRole,
		&organization.CreatedAt,
		&organization.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get personal organization for user: %w", err)
	}
	return organization, nil
}

func (db *DB) CreateOrganization(organization *Organization) error {
	if organization == nil {
		return fmt.Errorf("create organization: organization is nil")
	}
	if strings.TrimSpace(organization.ID) == "" {
		organization.ID = NewID()
	}
	organization.Name = strings.TrimSpace(organization.Name)
	if organization.Name == "" {
		return fmt.Errorf("create organization: name is required")
	}
	if strings.TrimSpace(organization.Slug) == "" {
		slug, err := db.reserveUniqueOrganizationSlug(slugifyOrganizationName(organization.Name))
		if err != nil {
			return fmt.Errorf("create organization: %w", err)
		}
		organization.Slug = slug
	}

	personalOwner := any(nil)
	if organization.PersonalOwnerUserID != "" {
		personalOwner = organization.PersonalOwnerUserID
	}

	_, err := db.Exec(
		`INSERT INTO organizations (id, name, slug, is_personal, personal_owner_user_id)
		 VALUES (?, ?, ?, ?, ?)`,
		organization.ID,
		organization.Name,
		organization.Slug,
		organization.IsPersonal,
		personalOwner,
	)
	if err != nil {
		return fmt.Errorf("create organization: %w", err)
	}
	return nil
}

func (db *DB) AddOrganizationMember(organizationID, userID, role string) error {
	if err := upsertOrganizationMember(db.DB, organizationID, userID, role); err != nil {
		return fmt.Errorf("add organization member: %w", err)
	}
	return nil
}

type membershipExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func upsertOrganizationMember(q membershipExecer, organizationID, userID, role string) error {
	role = normalizeOrganizationRole(role)
	_, err := q.Exec(
		`INSERT INTO organization_memberships (organization_id, user_id, role)
		 VALUES (?, ?, ?)
		 ON CONFLICT(organization_id, user_id) DO UPDATE SET
		   role = CASE
		     WHEN organization_memberships.role = 'owner' OR excluded.role = 'owner' THEN 'owner'
		     ELSE 'member'
		   END`,
		organizationID, userID, role,
	)
	return err
}

func (db *DB) EnsurePersonalOrganization(user *User) (*Organization, error) {
	if user == nil {
		return nil, fmt.Errorf("ensure personal organization: user is nil")
	}

	organization, err := db.GetPersonalOrganizationForUser(user.ID)
	if err != nil {
		return nil, err
	}
	if organization != nil {
		if organization.MembershipRole == "" {
			if err := db.AddOrganizationMember(organization.ID, user.ID, OrganizationRoleOwner); err != nil {
				return nil, err
			}
			organization.MembershipRole = OrganizationRoleOwner
		}
		return organization, nil
	}

	organization = &Organization{
		Name:                fmt.Sprintf("%s Personal", user.Username),
		IsPersonal:          true,
		PersonalOwnerUserID: user.ID,
		MembershipRole:      OrganizationRoleOwner,
	}

	slug, err := db.reserveUniqueOrganizationSlug(slugifyOrganizationName(user.Username + "-personal"))
	if err != nil {
		return nil, err
	}
	organization.Slug = slug

	if err := db.CreateOrganization(organization); err != nil {
		return nil, err
	}
	if err := db.AddOrganizationMember(organization.ID, user.ID, OrganizationRoleOwner); err != nil {
		return nil, err
	}
	return organization, nil
}

func (db *DB) reserveUniqueOrganizationSlug(base string) (string, error) {
	slug := base
	if slug == "" {
		slug = "organization"
	}

	for attempt := 0; attempt < 10; attempt++ {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM organizations WHERE slug = ?`, slug).Scan(&count); err != nil {
			return "", fmt.Errorf("check organization slug: %w", err)
		}
		if count == 0 {
			return slug, nil
		}
		suffix := strings.ToLower(NewID())
		if len(suffix) > 6 {
			suffix = suffix[len(suffix)-6:]
		}
		slug = fmt.Sprintf("%s-%s", base, suffix)
	}

	return "", fmt.Errorf("failed to reserve organization slug")
}

func slugifyOrganizationName(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = organizationSlugRegex.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	if normalized == "" {
		return "organization"
	}
	return normalized
}

func normalizeOrganizationRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case OrganizationRoleOwner:
		return OrganizationRoleOwner
	default:
		return OrganizationRoleMember
	}
}
