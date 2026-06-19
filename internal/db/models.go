package db

import (
	"crypto/rand"
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
	DisplayOrder        int       `json:"display_order"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type OrganizationMembership struct {
	OrganizationID string    `json:"organization_id"`
	UserID         string    `json:"user_id"`
	Role           string    `json:"role"`
	CreatedAt      time.Time `json:"created_at"`
}

type Group struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Slug                string    `json:"slug"`
	IsDefault           bool      `json:"is_default"`
	PersonalOwnerUserID string    `json:"personal_owner_user_id,omitempty"`
	MembershipRole      string    `json:"membership_role"`
	ProjectCount        int       `json:"project_count"`
	DisplayOrder        int       `json:"display_order"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type GroupCreate struct {
	Name       string
	OwnerID    string
	ProjectIDs []string
}

// App is a bundle of projects inside a workspace (organization). Projects link
// to it via projects.app_id (nullable; NULL = standalone). DeployOrdered is an
// attribute of the entity, consumed only by the coordinated-deploy phase.
type App struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	DeployOrdered  bool      `json:"deploy_ordered"`
	DisplayOrder   int       `json:"display_order"`
	ProjectCount   int       `json:"project_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// AppCreate is the input to CreateApp.
type AppCreate struct {
	OrganizationID string
	Name           string
	ProjectIDs     []string
}

type GroupMember struct {
	GroupID   string    `json:"group_id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	AvatarURL string    `json:"avatar_url"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type GroupInvite struct {
	ID                string       `json:"id"`
	GroupID           string       `json:"group_id"`
	GroupName         string       `json:"group_name,omitempty"`
	GithubUsername    string       `json:"github_username"`
	Role              string       `json:"role"`
	InvitedByUserID   string       `json:"invited_by_user_id"`
	InvitedByUsername string       `json:"invited_by_username,omitempty"`
	Status            string       `json:"status"`
	RespondedAt       NullableTime `json:"responded_at"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

type GroupInviteCreate struct {
	GroupID         string
	GithubUsername  string
	Role            string
	InvitedByUserID string
}

type RefreshSession struct {
	ID         string       `json:"id"`
	UserID     string       `json:"user_id"`
	TokenHash  string       `json:"-"`
	ExpiresAt  time.Time    `json:"expires_at"`
	LastUsedAt NullableTime `json:"last_used_at"`
	RevokedAt  NullableTime `json:"revoked_at"`
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

type APIToken struct {
	ID         string       `json:"id"`
	UserID     string       `json:"user_id"`
	Name       string       `json:"name"`
	TokenHash  string       `json:"-"`
	LastUsedAt NullableTime `json:"last_used_at"`
	ExpiresAt  NullableTime `json:"expires_at"`
	RevokedAt  NullableTime `json:"revoked_at"`
	CreatedAt  time.Time    `json:"created_at"`
}

// PushSubscription is one Web Push target (browser/device) for a user.
// The endpoint is returned to the owning client so it can recognize "this
// device"; p256dh/auth are crypto material and never serialized.
type PushSubscription struct {
	ID                   string    `json:"id"`
	UserID               string    `json:"user_id"`
	Endpoint             string    `json:"endpoint"`
	P256dh               string    `json:"-"`
	Auth                 string    `json:"-"`
	DeviceLabel          string    `json:"device_label"`
	NotifyDeployOutcomes bool      `json:"notify_deploy_outcomes"`
	NotifyBuildStarts    bool      `json:"notify_build_starts"`
	NotifySSLIssues      bool      `json:"notify_ssl_issues"`
	CreatedAt            time.Time `json:"created_at"`
}

type Project struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	GithubRepo        string `json:"github_repo"`
	GithubOwner       string `json:"github_owner"`
	Branch            string `json:"branch"`
	UserID            string `json:"user_id"`
	OrganizationID    string `json:"organization_id"`
	OrganizationName  string `json:"organization_name,omitempty"`
	AppID             string `json:"app_id,omitempty"` // empty = not in an app
	Framework         string `json:"framework"`
	PackageManager    string `json:"package_manager"`
	RootDirectory     string `json:"root_directory"`
	OutputDirectory   string `json:"output_directory"`
	BuildCommand      string `json:"build_command"`
	InstallCommand    string `json:"install_command"`
	NodeVersion       string `json:"node_version"`
	Port              int    `json:"port"`
	HostNetworkAccess bool   `json:"host_network_access"`
	DataVolumeEnabled bool   `json:"data_volume_enabled"`
	DataMountPath     string `json:"data_mount_path"`
	ResourceTier      string `json:"resource_tier"`
	// StartCommand and HealthPath drive the generated node-api Dockerfile's
	// CMD and HEALTHCHECK. Empty values mean "use the runtime default" — see
	// projectconfig.DefaultStartCommand and DefaultHealthPath.
	StartCommand string `json:"start_command"`
	HealthPath   string `json:"health_path"`
	// BuildFilterEnabled opts a project into changed-path build filtering (the
	// monorepo fan-out fix). WatchPaths is a JSON-encoded list of globs for
	// shared dependencies outside RootDirectory. Both inert when filtering is off.
	BuildFilterEnabled bool      `json:"build_filter_enabled"`
	WatchPaths         []string  `json:"watch_paths"`
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

// IsEnvironmentProtected reports whether the given environment ("preview" or
// "production") has a stored password. The stored value is encrypted, so only
// presence is meaningful here — callers needing the plaintext must decrypt.
func (p *Project) IsEnvironmentProtected(environment string) bool {
	switch environment {
	case "preview":
		return p.PreviewPassword != ""
	case "production":
		return p.ProductionPassword != ""
	}
	return false
}

// ServiceType identifies a sidecar service kind. v1 ships postgres only;
// "redis", "mysql" etc. are reserved for follow-up plans.
type ServiceType string

const (
	ServiceTypePostgres ServiceType = "postgres"
)

// ServiceStatus mirrors the CHECK constraint in migration 023.
type ServiceStatus string

const (
	ServiceStatusPending ServiceStatus = "pending"
	ServiceStatusRunning ServiceStatus = "running"
	ServiceStatusStopped ServiceStatus = "stopped"
	ServiceStatusFailed  ServiceStatus = "failed"
)

// ProjectService is one row of project_services. db_password_encrypted is
// AES-256-GCM ciphertext (decrypt via crypto.Encryptor); never expose in JSON.
type ProjectService struct {
	ID                  string        `json:"id"`
	ProjectID           string        `json:"project_id"`
	Environment         string        `json:"environment"`
	ServiceType         ServiceType   `json:"service_type"`
	Image               string        `json:"image"`
	DBName              string        `json:"db_name"`
	DBUser              string        `json:"db_user"`
	DBPasswordEncrypted string        `json:"-"` // ciphertext, never in JSON
	HostPort            int           `json:"host_port"`
	ConfigJSON          string        `json:"config_json,omitempty"`
	Status              ServiceStatus `json:"status"`
	LastStartedAt       NullableTime  `json:"last_started_at"`
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`
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
	LastEventAt      NullableTime            `json:"last_event_at"`
	VerifiedAt       NullableTime            `json:"verified_at"`
	LastError        string                  `json:"last_error"`
	CreatedAt        time.Time               `json:"created_at"`
	UpdatedAt        time.Time               `json:"updated_at"`
}

type EmailProvider string

const (
	EmailProviderWebglobe EmailProvider = "webglobe"
	EmailProviderSMTP     EmailProvider = "smtp"
)

type EmailSMTPSecurity string

const (
	EmailSMTPSecurityStartTLS EmailSMTPSecurity = "starttls"
	EmailSMTPSecurityTLS      EmailSMTPSecurity = "tls"
	EmailSMTPSecurityNone     EmailSMTPSecurity = "none"
)

type EmailRecaptchaMode string

const (
	EmailRecaptchaModeV3 EmailRecaptchaMode = "v3"
)

type EmailStatus string

const (
	EmailStatusNotConfigured  EmailStatus = "not_configured"
	EmailStatusReadyToInstall EmailStatus = "ready_to_install"
	EmailStatusSMTPTested     EmailStatus = "smtp_tested"
	EmailStatusError          EmailStatus = "error"
)

type ProjectEmailSettings struct {
	ProjectID               string             `json:"project_id"`
	Provider                EmailProvider      `json:"provider"`
	SMTPHost                string             `json:"smtp_host"`
	SMTPPort                int                `json:"smtp_port"`
	SMTPSecurity            EmailSMTPSecurity  `json:"smtp_security"`
	SMTPUser                string             `json:"smtp_user"`
	EmailFrom               string             `json:"email_from"`
	EmailFromName           string             `json:"email_from_name"`
	ContactEmailTo          string             `json:"contact_email_to"`
	RecaptchaSiteKey        string             `json:"recaptcha_site_key"`
	RecaptchaMode           EmailRecaptchaMode `json:"recaptcha_mode"`
	RecaptchaScoreThreshold float64            `json:"recaptcha_score_threshold"`
	Status                  EmailStatus        `json:"status"`
	LastTestedAt            NullableTime       `json:"last_tested_at"`
	LastTestError           string             `json:"last_test_error"`
	CreatedAt               time.Time          `json:"created_at"`
	UpdatedAt               time.Time          `json:"updated_at"`
}

type Deployment struct {
	ID                  string       `json:"id"`
	ProjectID           string       `json:"project_id"`
	Environment         string       `json:"environment"`
	PreviewInstanceID   string       `json:"preview_instance_id,omitempty"`
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
	FinishedAt          NullableTime `json:"finished_at"`
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
	ID                string       `json:"id"`
	ProjectID         string       `json:"project_id"`
	PreviewInstanceID string       `json:"preview_instance_id,omitempty"`
	DomainName        string       `json:"domain"`
	Environment       string       `json:"environment"`
	IsAuto            bool         `json:"is_auto"`
	IsPrimary         bool         `json:"is_primary"`
	DNSVerified       bool         `json:"dns_verified"`
	SSLStatus         string       `json:"ssl_status"`
	SSLExpiresAt      NullableTime `json:"ssl_expires_at,omitempty"`
	CreatedAt         time.Time    `json:"created_at"`
}

type DomainProvisionTarget struct {
	ProjectID              string
	ProjectName            string
	DomainName             string
	Environment            string
	PreviewInstanceID      string
	PreviewBranch          string
	PreviewBranchSlug      string
	PreviewInstanceDefault bool
	PasswordProtected      bool
	Port                   int
}

// ProductionMonitorTarget is one external health-check target: the primary
// SSL-active production domain of an active project. Consumed by the
// /api/monitoring/targets Prometheus http_sd endpoint. HealthPath is carried
// for a future health-endpoint probe; v1 probes the domain root.
type ProductionMonitorTarget struct {
	ProjectName string
	DomainName  string
	Protected   bool
	HealthPath  string
}

type PreviewInstance struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	Branch     string    `json:"branch"`
	BranchSlug string    `json:"branch_slug"`
	IsDefault  bool      `json:"is_default"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
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

// AppVariable is an app-scoped env var or secret (mirrors ProjectVariable but
// owned by an app). Layered underneath each member's project variables at
// deploy time. Value is encrypted at rest and masked in API responses.
type AppVariable struct {
	ID          string       `json:"id"`
	AppID       string       `json:"app_id"`
	Environment string       `json:"environment"`
	Kind        VariableKind `json:"kind"`
	Key         string       `json:"key"`
	Value       string       `json:"value"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type EnvVariable = ProjectVariable

type AutoBuildConfig struct {
	ID                    string    `json:"id"`
	ProjectID             string    `json:"project_id"`
	Enabled               bool      `json:"enabled"`
	ProductionBranch      string    `json:"production_branch"`
	PreviewBranches       string    `json:"preview_branches"`
	AutoProductionEnabled bool      `json:"auto_production_enabled"`
	WebhookID             *int64    `json:"webhook_id,omitempty"`
	WebhookSecret         string    `json:"-"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type WebhookEvent struct {
	ID                int64     `json:"id"`
	ProjectID         string    `json:"project_id"`
	GithubDeliveryID  string    `json:"github_delivery_id"`
	EventType         string    `json:"event_type"`
	Environment       string    `json:"environment"`
	PreviewInstanceID string    `json:"preview_instance_id,omitempty"`
	Branch            string    `json:"branch"`
	CommitSHA         string    `json:"commit_sha"`
	CommitMessage     string    `json:"commit_message"`
	Pusher            string    `json:"pusher"`
	DeploymentID      string    `json:"deployment_id,omitempty"`
	Status            string    `json:"status"`
	ErrorMessage      *string   `json:"error_message,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type ProjectWithLatestDeployment struct {
	Project
	LatestDeploymentID             *string    `json:"latest_deployment_id,omitempty"`
	LatestDeploymentStatus         *string    `json:"latest_deployment_status,omitempty"`
	LatestDeploymentBranch         *string    `json:"latest_deployment_branch,omitempty"`
	LatestDeploymentCommitSHA      *string    `json:"latest_deployment_commit_sha,omitempty"`
	LatestDeploymentCommitMsg      *string    `json:"latest_deployment_commit_message,omitempty"`
	LatestDeploymentCreatedAt      *time.Time `json:"latest_deployment_created_at,omitempty"`
	LatestDeploymentEnvironment    *string    `json:"latest_deployment_environment,omitempty"`
	LatestDeploymentScreenshotPath *string    `json:"latest_deployment_screenshot_path,omitempty"`
}

type DeploymentWithUser struct {
	Deployment
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}

type PreviewInstanceSummary struct {
	PreviewInstance
	DomainName              string     `json:"domain"`
	LatestDeploymentID      *string    `json:"latest_deployment_id,omitempty"`
	LatestDeploymentStatus  *string    `json:"latest_deployment_status,omitempty"`
	LatestDeploymentSHA     *string    `json:"latest_deployment_commit_sha,omitempty"`
	LatestDeploymentMessage *string    `json:"latest_deployment_commit_message,omitempty"`
	LatestDeploymentAt      *time.Time `json:"latest_deployment_created_at,omitempty"`
	LatestScreenshotPath    *string    `json:"latest_deployment_screenshot_path,omitempty"`
	// VolumeExists/VolumeSizeBytes describe this branch's isolated data volume
	// (deployik-{project}-preview-{slug}-data). Populated by the handler from a
	// Docker /system/df lookup; false/0 when the project has no data volumes or
	// the branch hasn't created one yet.
	VolumeExists    bool  `json:"volume_exists"`
	VolumeSizeBytes int64 `json:"volume_size_bytes"`
}

type DeploymentListResponse struct {
	Deployments []DeploymentWithUser `json:"deployments"`
	Total       int                  `json:"total"`
}

type DeploymentFilter struct {
	ProjectID         string
	Branch            string
	Environment       string
	PreviewInstanceID string
	Status            string
	TriggeredBy       string
	From              string
	To                string
	Limit             int
	Offset            int
}
