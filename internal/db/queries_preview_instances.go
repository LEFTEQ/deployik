package db

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
)

// PreviewDomainSuffix is the DNS suffix for auto-generated preview domains
// (e.g. my-app.preview.example.com). It carries a neutral default so tests and
// DEV_MODE work out of the box, and is overwritten exactly once at startup from
// BASE_DOMAIN / PREVIEW_DOMAIN_SUFFIX (see cmd/server/main.go). Treat it as
// set-once configuration, not a runtime-mutable value.
var PreviewDomainSuffix = "preview.example.com"

const MaxDNSLabelLength = 63

func NormalizePreviewBranchSlug(projectName, branch string) (string, error) {
	maxBranchLen := MaxDNSLabelLength - len(projectName) - 1
	if maxBranchLen <= 0 {
		return "", fmt.Errorf("project name is too long for branch preview domains")
	}

	base := sanitizePreviewBranchSlug(branch)
	if len(base) <= maxBranchLen {
		return base, nil
	}

	suffix := shortBranchHash(branch)
	if maxBranchLen > len(suffix)+1 {
		keep := maxBranchLen - len(suffix) - 1
		base = strings.Trim(base[:keep], "-")
		if base == "" {
			return suffix[:maxBranchLen], nil
		}
		return base + "-" + suffix, nil
	}

	return strings.Trim(base[:maxBranchLen], "-"), nil
}

func DefaultPreviewDomain(projectName string) string {
	return projectName + "." + PreviewDomainSuffix
}

func PreviewBranchDomain(projectName, branchSlug string) string {
	return projectName + "-" + branchSlug + "." + PreviewDomainSuffix
}

func PreviewContainerName(projectName string, instance *PreviewInstance) string {
	if instance == nil || instance.IsDefault {
		return capContainerName(fmt.Sprintf("deployik-%s-preview", projectName))
	}
	return capContainerName(fmt.Sprintf("deployik-%s-preview-%s", projectName, instance.BranchSlug))
}

func DeploymentContainerName(projectName, environment string, instance *PreviewInstance) string {
	if environment == "preview" {
		return PreviewContainerName(projectName, instance)
	}
	return capContainerName(fmt.Sprintf("deployik-%s-%s", projectName, environment))
}

// capContainerName keeps a container name within the DNS label limit so it stays
// resolvable as an nginx upstream host / Docker network alias. Branch slugs are
// already capped for the *domain* label (NormalizePreviewBranchSlug), but the
// "deployik-…-preview-" scaffolding the container name wraps around the slug adds
// ~18 characters on top — enough to push a long preview-instance name past 63,
// at which point Docker's embedded DNS refuses to resolve the name and nginx's
// proxy_pass to that upstream returns 502 even though the container is healthy.
// Names already within the limit are returned unchanged (no behaviour change for
// the common case); over-long names are truncated and given a short deterministic
// hash suffix so distinct inputs keep distinct, stable names across deploys.
func capContainerName(name string) string {
	if len(name) <= MaxDNSLabelLength {
		return name
	}
	suffix := shortBranchHash(name)
	keep := MaxDNSLabelLength - len(suffix) - 1
	trimmed := strings.Trim(name[:keep], "-")
	if trimmed == "" {
		return suffix
	}
	return trimmed + "-" + suffix
}

func DeploymentVolumeName(projectName, environment string, instance *PreviewInstance) string {
	return DeploymentContainerName(projectName, environment, instance) + "-data"
}

func (p PreviewInstance) AutoDomain(projectName string) string {
	return PreviewBranchDomain(projectName, p.BranchSlug)
}

func sanitizePreviewBranchSlug(branch string) string {
	value := strings.ToLower(strings.TrimSpace(branch))
	if value == "" {
		value = "branch"
	}

	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune('-')
			lastDash = true
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}

	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "branch"
	}
	return slug
}

func shortBranchHash(branch string) string {
	sum := sha1.Sum([]byte(branch))
	return hex.EncodeToString(sum[:])[:8]
}

func slugWithSuffix(base, suffix string, maxLen int) string {
	if maxLen <= len(suffix)+1 {
		if len(suffix) > maxLen {
			return suffix[:maxLen]
		}
		return suffix
	}
	keep := maxLen - len(suffix) - 1
	base = strings.Trim(base, "-")
	if len(base) > keep {
		base = strings.Trim(base[:keep], "-")
	}
	if base == "" {
		return suffix
	}
	return base + "-" + suffix
}

func scanPreviewInstance(row interface {
	Scan(...any) error
}, p *PreviewInstance) error {
	return row.Scan(&p.ID, &p.ProjectID, &p.Branch, &p.BranchSlug, &p.IsDefault, &p.Status, &p.CreatedAt, &p.UpdatedAt)
}

func (db *DB) GetPreviewInstanceByID(id string) (*PreviewInstance, error) {
	p := &PreviewInstance{}
	err := scanPreviewInstance(db.QueryRow(
		`SELECT id, project_id, branch, branch_slug, is_default, status, created_at, updated_at
		 FROM preview_instances WHERE id = ?`,
		id,
	), p)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get preview instance by id: %w", err)
	}
	return p, nil
}

func (db *DB) GetPreviewInstanceForBranch(projectID, branch string) (*PreviewInstance, error) {
	p := &PreviewInstance{}
	err := scanPreviewInstance(db.QueryRow(
		`SELECT id, project_id, branch, branch_slug, is_default, status, created_at, updated_at
		 FROM preview_instances WHERE project_id = ? AND branch = ?`,
		projectID, branch,
	), p)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get preview instance for branch: %w", err)
	}
	return p, nil
}

func (db *DB) GetDefaultPreviewInstance(projectID string) (*PreviewInstance, error) {
	p := &PreviewInstance{}
	err := scanPreviewInstance(db.QueryRow(
		`SELECT id, project_id, branch, branch_slug, is_default, status, created_at, updated_at
		 FROM preview_instances
		 WHERE project_id = ? AND is_default = 1 AND status = 'active'
		 ORDER BY created_at ASC
		 LIMIT 1`,
		projectID,
	), p)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get default preview instance: %w", err)
	}
	return p, nil
}

func (db *DB) getPreviewInstanceBySlug(projectID, slug string) (*PreviewInstance, error) {
	p := &PreviewInstance{}
	err := scanPreviewInstance(db.QueryRow(
		`SELECT id, project_id, branch, branch_slug, is_default, status, created_at, updated_at
		 FROM preview_instances WHERE project_id = ? AND branch_slug = ?`,
		projectID, slug,
	), p)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get preview instance by slug: %w", err)
	}
	return p, nil
}

func (db *DB) GetOrCreatePreviewInstance(project *Project, branch string) (*PreviewInstance, error) {
	if project == nil {
		return nil, fmt.Errorf("project is required")
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = project.Branch
	}
	if branch == "" {
		branch = "main"
	}

	if existing, err := db.GetPreviewInstanceForBranch(project.ID, branch); err != nil {
		return nil, err
	} else if existing != nil {
		if existing.Status == "deleted" {
			if _, err := db.Exec(
				`UPDATE preview_instances SET status = 'active', updated_at = datetime('now') WHERE id = ?`,
				existing.ID,
			); err != nil {
				return nil, fmt.Errorf("reactivate preview instance: %w", err)
			}
			existing.Status = "active"
		}
		return existing, nil
	}

	baseSlug, err := NormalizePreviewBranchSlug(project.Name, branch)
	if err != nil {
		return nil, err
	}
	maxBranchLen := MaxDNSLabelLength - len(project.Name) - 1
	slug := baseSlug
	for attempt := 0; attempt < 20; attempt++ {
		existing, err := db.getPreviewInstanceBySlug(project.ID, slug)
		if err != nil {
			return nil, err
		}
		if existing == nil || existing.Branch == branch {
			break
		}
		if attempt == 0 {
			slug = slugWithSuffix(baseSlug, shortBranchHash(branch), maxBranchLen)
		} else {
			slug = slugWithSuffix(baseSlug, fmt.Sprintf("%s-%d", shortBranchHash(branch)[:6], attempt+1), maxBranchLen)
		}
	}

	defaultInstance, err := db.GetDefaultPreviewInstance(project.ID)
	if err != nil {
		return nil, err
	}
	isDefault := defaultInstance == nil && branch == project.Branch

	instance := &PreviewInstance{
		ID:         NewID(),
		ProjectID:  project.ID,
		Branch:     branch,
		BranchSlug: slug,
		IsDefault:  isDefault,
		Status:     "active",
	}
	_, err = db.Exec(
		`INSERT INTO preview_instances (id, project_id, branch, branch_slug, is_default, status)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		instance.ID, instance.ProjectID, instance.Branch, instance.BranchSlug, instance.IsDefault, instance.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("create preview instance: %w", err)
	}
	return db.GetPreviewInstanceByID(instance.ID)
}

func (db *DB) ListPreviewInstances(projectID string) ([]PreviewInstance, error) {
	rows, err := db.Query(
		`SELECT id, project_id, branch, branch_slug, is_default, status, created_at, updated_at
		 FROM preview_instances
		 WHERE project_id = ? AND status = 'active'
		 ORDER BY is_default DESC, updated_at DESC, created_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list preview instances: %w", err)
	}
	defer rows.Close()

	var instances []PreviewInstance
	for rows.Next() {
		var p PreviewInstance
		if err := scanPreviewInstance(rows, &p); err != nil {
			return nil, fmt.Errorf("scan preview instance: %w", err)
		}
		instances = append(instances, p)
	}
	return instances, rows.Err()
}

func (db *DB) ListPreviewInstanceSummaries(projectID string) ([]PreviewInstanceSummary, error) {
	rows, err := db.Query(
		`SELECT pi.id, pi.project_id, pi.branch, pi.branch_slug, pi.is_default, pi.status, pi.created_at, pi.updated_at,
		        COALESCE(d.domain, ''),
		        ld.id, ld.status, ld.commit_sha, ld.commit_message, ld.created_at, ld.screenshot_path
		 FROM preview_instances pi
		 LEFT JOIN domains d ON d.preview_instance_id = pi.id AND d.is_primary = 1
		 LEFT JOIN (
		     SELECT d1.*
		     FROM deployments d1
		     WHERE d1.environment = 'preview'
		       AND NOT EXISTS (
		           SELECT 1 FROM deployments d2
		           WHERE d2.project_id = d1.project_id
		             AND d2.preview_instance_id = d1.preview_instance_id
		             AND d2.environment = 'preview'
		             AND d2.created_at > d1.created_at
		       )
		 ) ld ON ld.project_id = pi.project_id AND ld.preview_instance_id = pi.id
		 WHERE pi.project_id = ? AND pi.status = 'active'
		 ORDER BY pi.is_default DESC, pi.updated_at DESC, pi.created_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list preview instance summaries: %w", err)
	}
	defer rows.Close()

	var summaries []PreviewInstanceSummary
	for rows.Next() {
		var s PreviewInstanceSummary
		var deploymentID, deploymentStatus, deploymentSHA, deploymentMessage, screenshotPath sql.NullString
		var deploymentAt sql.NullTime
		if err := rows.Scan(
			&s.ID, &s.ProjectID, &s.Branch, &s.BranchSlug, &s.IsDefault, &s.Status, &s.CreatedAt, &s.UpdatedAt,
			&s.DomainName,
			&deploymentID, &deploymentStatus, &deploymentSHA, &deploymentMessage, &deploymentAt, &screenshotPath,
		); err != nil {
			return nil, fmt.Errorf("scan preview instance summary: %w", err)
		}
		if deploymentID.Valid {
			s.LatestDeploymentID = &deploymentID.String
		}
		if deploymentStatus.Valid {
			s.LatestDeploymentStatus = &deploymentStatus.String
		}
		if deploymentSHA.Valid {
			s.LatestDeploymentSHA = &deploymentSHA.String
		}
		if deploymentMessage.Valid {
			s.LatestDeploymentMessage = &deploymentMessage.String
		}
		if deploymentAt.Valid {
			s.LatestDeploymentAt = &deploymentAt.Time
		}
		if screenshotPath.Valid {
			s.LatestScreenshotPath = &screenshotPath.String
		}
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}

func (db *DB) DeletePreviewInstance(projectID, id string) error {
	_, err := db.Exec(
		`UPDATE preview_instances
		 SET status = 'deleted', updated_at = datetime('now')
		 WHERE id = ? AND project_id = ? AND is_default = 0`,
		id, projectID,
	)
	if err != nil {
		return fmt.Errorf("delete preview instance: %w", err)
	}
	return nil
}

func (db *DB) EnsurePreviewAutoDomains(project *Project, instance *PreviewInstance) ([]Domain, error) {
	if project == nil || instance == nil {
		return nil, fmt.Errorf("project and preview instance are required")
	}

	type desiredDomain struct {
		name      string
		isPrimary bool
	}
	desired := []desiredDomain{{name: instance.AutoDomain(project.Name), isPrimary: !instance.IsDefault}}
	if instance.IsDefault {
		desired = []desiredDomain{
			{name: DefaultPreviewDomain(project.Name), isPrimary: true},
			{name: instance.AutoDomain(project.Name), isPrimary: false},
		}
	}

	created := make([]Domain, 0, len(desired))
	for _, item := range desired {
		existing, err := db.GetDomainByName(item.name)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			if existing.ProjectID != project.ID {
				return nil, fmt.Errorf("domain %s already belongs to another project", item.name)
			}
			if existing.PreviewInstanceID == "" {
				if err := db.UpdateDomainPreviewInstance(existing.ID, instance.ID); err != nil {
					return nil, err
				}
				existing.PreviewInstanceID = instance.ID
			}
			created = append(created, *existing)
			continue
		}

		domain := Domain{
			ProjectID:         project.ID,
			PreviewInstanceID: instance.ID,
			DomainName:        item.name,
			Environment:       "preview",
			IsAuto:            true,
			IsPrimary:         item.isPrimary,
			SSLStatus:         "pending",
		}
		if err := db.CreateDomain(&domain); err != nil {
			return nil, err
		}
		created = append(created, domain)
	}

	return created, nil
}
