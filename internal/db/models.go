package db

import (
	"crypto/rand"
	"database/sql"
	"time"

	"github.com/oklog/ulid/v2"
)

// NewID generates a new ULID.
func NewID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
}

type User struct {
	ID          string    `json:"id"`
	GithubID    int64     `json:"github_id"`
	Username    string    `json:"username"`
	AvatarURL   string    `json:"avatar_url"`
	GithubToken string    `json:"-"` // never expose in JSON
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}

type RefreshSession struct {
	ID         string       `json:"id"`
	UserID     string       `json:"user_id"`
	TokenHash  string       `json:"-"`
	ExpiresAt  time.Time    `json:"expires_at"`
	LastUsedAt sql.NullTime `json:"last_used_at"`
	RevokedAt  sql.NullTime `json:"revoked_at"`
	CreatedAt  time.Time    `json:"created_at"`
}

type AuditLog struct {
	ID           int64     `json:"id"`
	UserID       string    `json:"user_id"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	ProjectID    string    `json:"project_id"`
	DeploymentID string    `json:"deployment_id"`
	Metadata     string    `json:"metadata"`
	CreatedAt    time.Time `json:"created_at"`
}

type Project struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	GithubRepo      string    `json:"github_repo"`
	GithubOwner     string    `json:"github_owner"`
	Branch          string    `json:"branch"`
	UserID          string    `json:"user_id"`
	Framework       string    `json:"framework"`
	RootDirectory   string    `json:"root_directory"`
	OutputDirectory string    `json:"output_directory"`
	BuildCommand    string    `json:"build_command"`
	InstallCommand  string    `json:"install_command"`
	NodeVersion     string    `json:"node_version"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Deployment struct {
	ID            string       `json:"id"`
	ProjectID     string       `json:"project_id"`
	Environment   string       `json:"environment"`
	CommitSHA     string       `json:"commit_sha"`
	CommitMessage string       `json:"commit_message"`
	Branch        string       `json:"branch"`
	Status        string       `json:"status"`
	ContainerID   string       `json:"container_id"`
	ContainerName string       `json:"container_name"`
	ImageTag      string       `json:"image_tag"`
	BuildDuration int          `json:"build_duration"`
	TriggeredBy   string       `json:"triggered_by"`
	ErrorMessage  string       `json:"error_message,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
	FinishedAt    sql.NullTime `json:"finished_at"`
}

type BuildLog struct {
	ID           int64     `json:"id"`
	DeploymentID string    `json:"deployment_id"`
	LineNumber   int       `json:"line_number"`
	Content      string    `json:"content"`
	Stream       string    `json:"stream"`
	Timestamp    time.Time `json:"timestamp"`
}

type Domain struct {
	ID           string       `json:"id"`
	ProjectID    string       `json:"project_id"`
	DomainName   string       `json:"domain"`
	Environment  string       `json:"environment"`
	IsAuto       bool         `json:"is_auto"`
	DNSVerified  bool         `json:"dns_verified"`
	SSLStatus    string       `json:"ssl_status"`
	SSLExpiresAt sql.NullTime `json:"ssl_expires_at,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
}

type VariableKind string

const (
	VariableKindEnv    VariableKind = "env"
	VariableKindSecret VariableKind = "secret"
)

type ProjectVariable struct {
	ID          string       `json:"id"`
	ProjectID   string       `json:"project_id"`
	Environment string       `json:"environment"`
	Kind        VariableKind `json:"kind"`
	Key         string       `json:"key"`
	Value       string       `json:"value"` // encrypted at rest, masked in API responses
	CreatedAt   time.Time    `json:"created_at"`
}

type EnvVariable = ProjectVariable
