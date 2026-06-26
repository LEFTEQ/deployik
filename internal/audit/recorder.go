package audit

import (
	"encoding/json"
	"log"

	"github.com/lefteq/lovinka-deployik/internal/db"
)

type Entry struct {
	UserID       string
	Action       string
	ResourceType string
	ResourceID   string
	ProjectID    string
	DeploymentID string
	Metadata     map[string]any
}

type Recorder struct {
	DB *db.DB
}

func (r *Recorder) Record(entry Entry) {
	if r == nil || r.DB == nil || entry.Action == "" {
		return
	}

	metadata := "{}"
	if len(entry.Metadata) > 0 {
		payload, err := json.Marshal(entry.Metadata)
		if err != nil {
			log.Printf("Audit metadata marshal failed for %s: %v", entry.Action, err)
			return
		}
		metadata = string(payload)
	}

	if err := r.DB.CreateAuditLog(&db.AuditLog{
		UserID:       entry.UserID,
		Action:       entry.Action,
		ResourceType: entry.ResourceType,
		ResourceID:   entry.ResourceID,
		ProjectID:    entry.ProjectID,
		DeploymentID: entry.DeploymentID,
		Metadata:     metadata,
	}); err != nil {
		log.Printf("Audit log write failed for %s: %v", entry.Action, err)
	}
}
