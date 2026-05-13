package build

import (
	"context"
	"log"
	"time"
)

// DefaultImageRetention controls how many recent images we keep per
// (project, environment, preview_instance). Older deployment rows keep their
// database history; only their image tags are pruned from the local Docker
// daemon. Tuned to 3 so a manual rollback always has the previous live + one
// extra to fall back to.
const DefaultImageRetention = 3

// PruneOldImages removes Docker images from deployments that are older than
// the most recent `keep` for the given target (project + environment +
// preview_instance). The current deployment's imageTag is always preserved
// defensively in case the caller hasn't committed status='live' yet. Errors
// are logged and swallowed — image retention is best-effort.
//
// Intended to run on a goroutine right after a successful blue-green swap.
// Volumes, build logs, and the deployments rows themselves are untouched.
func (p *Pipeline) PruneOldImages(projectID, environment, previewInstanceID, currentImageTag string, keep int) {
	if p == nil || p.DB == nil || p.Docker == nil {
		return
	}
	if keep <= 0 {
		keep = DefaultImageRetention
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Use COALESCE so preview rows with NULL preview_instance_id match the
	// empty-string production case cleanly.
	rows, err := p.DB.Query(
		`SELECT image_tag FROM deployments
		 WHERE project_id = ?
		   AND environment = ?
		   AND COALESCE(preview_instance_id, '') = ?
		   AND image_tag != ''
		 ORDER BY created_at DESC`,
		projectID, environment, previewInstanceID,
	)
	if err != nil {
		log.Printf("retention: list deployments for project %s/%s: %v", projectID, environment, err)
		return
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			log.Printf("retention: scan image_tag: %v", err)
			return
		}
		if tag != currentImageTag {
			tags = append(tags, tag)
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("retention: iter image_tag: %v", err)
		return
	}

	// Keep the most recent (keep-1) old images — current already counts as the
	// first slot. So we remove everything past index keep-1.
	keepOld := keep - 1
	if keepOld < 0 {
		keepOld = 0
	}
	if len(tags) <= keepOld {
		return
	}
	for _, tag := range tags[keepOld:] {
		if err := p.Docker.RemoveImage(ctx, tag); err != nil {
			// errdefs.NotFound is the common case (already pruned by the weekly
			// timer). Logged so an operator can spot wedged removals.
			log.Printf("retention: prune image %s for project %s/%s: %v",
				tag, projectID, environment, err)
		}
	}
}
