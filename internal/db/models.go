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

type Organization struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Slug                string    `json:"slug"`
	IsPersonal          bool      `json:"is_personal"`
	PersonalOwnerUserID string    `json:"personal_owner_user_id,omitempty"`
	MembershipRole      string    `json:"membership_role"`
	ProjectCount        int       `json:"project_count"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type OrganizationMembership struct {
	OrganizationID string    `json:"organization_id"`
	UserID         string    `json:"user_id"`
	Role           string    `json:"role"`
	CreatedAt      time.Time `json:"created_at"`
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
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	GithubRepo         string    `json:"github_repo"`
	GithubOwner        string    `json:"github_owner"`
	Branch             string    `json:"branch"`
	UserID             string    `json:"user_id"`
	OrganizationID     string    `json:"organization_id"`
	OrganizationName   string    `json:"organization_name,omitempty"`
	Framework          string    `json:"framework"`
	PackageManager     string    `json:"package_manager"`
	RootDirectory      string    `json:"root_directory"`
	OutputDirectory    string    `json:"output_directory"`
	BuildCommand       string    `json:"build_command"`
	InstallCommand     string    `json:"install_command"`
	NodeVersion        string    `json:"node_version"`
	Port               int       `json:"port"`
	HostNetworkAccess  bool      `json:"host_network_access"`
	DataVolumeEnabled  bool      `json:"data_volume_enabled"`
	DataMountPath      string    `json:"data_mount_path"`
	Status             string    `json:"status"`
	PreviewPassword    string    `json:"-"` // encrypted, never expose in JSON
	ProductionPassword string    `json:"-"` // encrypted, never expose in JSON
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	// Per-environment timestamps of the most recent deployment that reached
	// status='live'. Nil when the environment has never had a live deploy.
	// Drives the "variables changed since last deploy" UI signal.
	LatestPreviewDeployAt    *time.Time `json:"latest_preview_deploy_at,omitempty"`
	LatestProductionDeployAt *time.Time `json:"latest_production_deploy_at,omitempty"`
}

type AnalyticsTrackingMode string

const (
	AnalyticsTrackingModeAIInstall AnalyticsTrackingMode = "ai_install"
	AnalyticsTrackingModeManual    AnalyticsTrackingMode = "manual"
	AnalyticsTrackingModeDisabled  AnalyticsTrackingMode = "disabled"
)

type AnalyticsAudienceStatus string

const (
	AnalyticsAudienceStatusProvisioning   AnalyticsAudienceStatus = "provisioning"
	AnalyticsAudienceStatusReadyToInstall AnalyticsAudienceStatus = "ready_to_install"
	AnalyticsAudienceStatusWaitingForData AnalyticsAudienceStatus = "waiting_for_data"
	AnalyticsAudienceStatusReceivingData  AnalyticsAudienceStatus = "receiving_data"
	AnalyticsAudienceStatusStale          AnalyticsAudienceStatus = "stale"
	AnalyticsAudienceStatusUnavailable    AnalyticsAudienceStatus = "unavailable"
	AnalyticsAudienceStatusError          AnalyticsAudienceStatus = "error"
)

type ProjectAnalytics struct {
	ProjectID        string                  `json:"project_id"`
	AudienceEnabled  bool                    `json:"audience_enabled"`
	TrackingMode     AnalyticsTrackingMode   `json:"tracking_mode"`
	AudienceStatus   AnalyticsAudienceStatus `json:"audience_status"`
	UmamiWebsiteID   string                  `json:"umami_website_id"`
	UmamiWebsiteName string                  `json:"umami_website_name"`
	LastEventAt      sql.NullTime            `json:"last_event_at"`
	VerifiedAt       sql.NullTime            `json:"verified_at"`
	LastError        string                  `json:"last_error"`
	CreatedAt        time.Time               `json:"created_at"`
	UpdatedAt        time.Time               `json:"updated_at"`
}

type Deployment struct {
	ID                  string       `json:"id"`
	ProjectID           string       `json:"project_id"`
	Environment         string       `json:"environment"`
	CommitSHA           string       `json:"commit_sha"`
	CommitMessage       string       `json:"commit_message"`
	Branch              string       `json:"branch"`
	Status              string       `json:"status"`
	ContainerID         string       `json:"container_id"`
	ContainerName       string       `json:"container_name"`
	ImageTag            string       `json:"image_tag"`
	BuildDuration       int          `json:"build_duration"`
	TriggeredBy         string       `json:"triggered_by"`
	TriggerSource       string       `json:"trigger_source"`
	TriggeredByUsername string       `json:"triggered_by_username"`
	ScreenshotPath      string       `json:"screenshot_path,omitempty"`
	ErrorMessage        string       `json:"error_message,omitempty"`
	CreatedAt           time.Time    `json:"created_at"`
	FinishedAt          sql.NullTime `json:"finished_at"`
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
	IsPrimary    bool         `json:"is_primary"`
	DNSVerified  bool         `json:"dns_verified"`
	SSLStatus    string       `json:"ssl_status"`
	SSLExpiresAt sql.NullTime `json:"ssl_expires_at,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
}

type DomainProvisionTarget struct {
	ProjectID         string
	ProjectName       string
	DomainName        string
	Environment       string
	PasswordProtected bool
	Port              int
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
	UpdatedAt   time.Time    `json:"updated_at"`
}

type EnvVariable = ProjectVariable

type AutoBuildConfig struct {
	ID               string    `json:"id"`
	ProjectID        string    `json:"project_id"`
	Enabled          bool      `json:"enabled"`
	ProductionBranch string    `json:"production_branch"`
	PreviewBranches  string    `json:"preview_branches"`
	WebhookID        *int64    `json:"webhook_id,omitempty"`
	WebhookSecret    string    `json:"-"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type WebhookEvent struct {
	ID               int64     `json:"id"`
	ProjectID        string    `json:"project_id"`
	GithubDeliveryID string    `json:"github_delivery_id"`
	EventType        string    `json:"event_type"`
	Branch           string    `json:"branch"`
	CommitSHA        string    `json:"commit_sha"`
	CommitMessage    string    `json:"commit_message"`
	Pusher           string    `json:"pusher"`
	DeploymentID     string    `json:"deployment_id,omitempty"`
	Status           string    `json:"status"`
	ErrorMessage     *string   `json:"error_message,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type ProjectWithLatestDeployment struct {
	Project
	LatestDeploymentID        *string    `json:"latest_deployment_id,omitempty"`
	LatestDeploymentStatus    *string    `json:"latest_deployment_status,omitempty"`
	LatestDeploymentBranch    *string    `json:"latest_deployment_branch,omitempty"`
	LatestDeploymentCommitSHA *string    `json:"latest_deployment_commit_sha,omitempty"`
	LatestDeploymentCommitMsg *string    `json:"latest_deployment_commit_message,omitempty"`
	LatestDeploymentCreatedAt *time.Time `json:"latest_deployment_created_at,omitempty"`
}

type DeploymentWithUser struct {
	Deployment
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}

type DeploymentListResponse struct {
	Deployments []DeploymentWithUser `json:"deployments"`
	Total       int                  `json:"total"`
}

type DeploymentFilter struct {
	ProjectID   string
	Branch      string
	Environment string
	Status      string
	TriggeredBy string
	From        string
	To          string
	Limit       int
	Offset      int
}
