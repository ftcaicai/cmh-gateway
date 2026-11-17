package migrations

import (
	"errors"

	"gorm.io/gorm"
)

// OEMSiteRecord contains only nonfinancial brand configuration. OwnerUserID
// references the official user; it does not confer a new role or relay identity.
type OEMSiteRecord struct {
	ID           string `gorm:"type:varchar(32);primaryKey"`
	OwnerUserID  int    `gorm:"not null;index"`
	Name         string `gorm:"type:varchar(100);not null"`
	SupportEmail string `gorm:"type:varchar(254);not null"`
	Description  string `gorm:"type:varchar(1000);not null"`
	Version      int64  `gorm:"not null"`
	CreatedAt    int64  `gorm:"not null"`
	UpdatedAt    int64  `gorm:"not null"`
}

func (OEMSiteRecord) TableName() string { return "cmh_oem_sites" }

// OEMSiteAuditRecord deliberately excludes contact details and free-form input.
// The configuration change and its audit row are committed in one transaction.
type OEMSiteAuditRecord struct {
	ID          string `gorm:"type:varchar(32);primaryKey"`
	SiteID      string `gorm:"type:varchar(32);not null;index"`
	ActorUserID int    `gorm:"not null"`
	Action      string `gorm:"type:varchar(32);not null"`
	Version     int64  `gorm:"not null"`
	CreatedAt   int64  `gorm:"not null"`
}

func (OEMSiteAuditRecord) TableName() string { return "cmh_oem_site_audits" }

// OEMCustomerInvitationRecord stores only a SHA256 token fingerprint. Tokens are
// purpose-bound, target-bound and expire; plaintext is returned once at creation.
type OEMCustomerInvitationRecord struct {
	ID           string `gorm:"type:varchar(32);primaryKey"`
	SiteID       string `gorm:"type:varchar(32);not null;index"`
	OwnerUserID  int    `gorm:"not null"`
	TargetUserID int    `gorm:"not null;index"`
	TokenHash    string `gorm:"type:varchar(64);not null;uniqueIndex"`
	CreatedAt    int64  `gorm:"not null"`
	ExpiresAt    int64  `gorm:"not null"`
	AcceptedAt   int64  `gorm:"not null"`
}

func (OEMCustomerInvitationRecord) TableName() string { return "cmh_oem_customer_invitations" }

// The primary key reserves one association globally, including after revoke.
// Revocation is not a gateway-user disable and never permits silent transfer.
type OEMCustomerRecord struct {
	UserID       int    `gorm:"primaryKey;autoIncrement:false"`
	SiteID       string `gorm:"type:varchar(32);not null;index"`
	OwnerUserID  int    `gorm:"not null"`
	InvitationID string `gorm:"type:varchar(32);not null"`
	Active       bool   `gorm:"not null"`
	JoinedAt     int64  `gorm:"not null"`
	RevokedAt    int64  `gorm:"not null"`
}

func (OEMCustomerRecord) TableName() string { return "cmh_oem_customers" }

type OEMCustomerAuditRecord struct {
	ID          string `gorm:"type:varchar(32);primaryKey"`
	SiteID      string `gorm:"type:varchar(32);not null;index"`
	UserID      int    `gorm:"not null"`
	ActorUserID int    `gorm:"not null"`
	Action      string `gorm:"type:varchar(32);not null"`
	CreatedAt   int64  `gorm:"not null"`
}

func (OEMCustomerAuditRecord) TableName() string { return "cmh_oem_customer_audits" }

// OEMCustomerNoteRecord is merchant-private and never projected into membership.
type OEMCustomerNoteRecord struct {
	UserID      int    `gorm:"primaryKey;autoIncrement:false"`
	SiteID      string `gorm:"type:varchar(32);not null;index"`
	OwnerUserID int    `gorm:"not null"`
	Note        string `gorm:"type:text;not null"`
	Version     int64  `gorm:"not null"`
	UpdatedAt   int64  `gorm:"not null"`
}

func (OEMCustomerNoteRecord) TableName() string { return "cmh_oem_customer_notes" }

type OEMTeamInvitationRecord struct {
	ExpectedMemberVersion int64  `gorm:"not null"`
	ID                    string `gorm:"type:varchar(32);primaryKey"`
	SiteID                string `gorm:"type:varchar(32);not null;index"`
	OwnerUserID           int    `gorm:"not null"`
	TargetUserID          int    `gorm:"not null"`
	TokenHash             string `gorm:"type:varchar(64);not null;uniqueIndex"`
	CreatedAt             int64  `gorm:"not null"`
	ExpiresAt             int64  `gorm:"not null"`
	AcceptedAt            int64  `gorm:"not null"`
}

func (OEMTeamInvitationRecord) TableName() string { return "cmh_oem_team_invitations" }

type OEMTeamMemberRecord struct {
	SiteID       string `gorm:"type:varchar(32);primaryKey"`
	UserID       int    `gorm:"primaryKey;autoIncrement:false"`
	OwnerUserID  int    `gorm:"not null"`
	InvitationID string `gorm:"type:varchar(32);not null"`
	Active       bool   `gorm:"not null"`
	Version      int64  `gorm:"not null"`
	JoinedAt     int64  `gorm:"not null"`
	RevokedAt    int64  `gorm:"not null"`
}

func (OEMTeamMemberRecord) TableName() string { return "cmh_oem_team_members" }

type OEMTeamAuditRecord struct {
	ID          string `gorm:"type:varchar(32);primaryKey"`
	SiteID      string `gorm:"type:varchar(32);not null;index"`
	UserID      int    `gorm:"not null"`
	ActorUserID int    `gorm:"not null"`
	Action      string `gorm:"type:varchar(32);not null"`
	CreatedAt   int64  `gorm:"not null"`
}

func (OEMTeamAuditRecord) TableName() string { return "cmh_oem_team_audits" }

// AuthScopeBindingRecord is the H8 durable scope of one official login flow
// (RecordID = token hash) or session (RecordID = SID). It holds no credential.
// Scope is "main" or "brand"; the site, origin and browser digest are empty for
// main. A flow or session without a row is rejected once H8 is enabled.
type AuthScopeBindingRecord struct {
	ID            int64  `gorm:"primaryKey"`
	Kind          string `gorm:"type:varchar(16);not null;uniqueIndex:idx_cmh_auth_scope_record,priority:1"`
	RecordID      string `gorm:"type:varchar(128);not null;uniqueIndex:idx_cmh_auth_scope_record,priority:2"`
	UserID        int    `gorm:"not null"`
	Scope         string `gorm:"type:varchar(8);not null"`
	SiteID        string `gorm:"type:varchar(32);not null"`
	Origin        string `gorm:"type:varchar(300);not null"`
	BrowserDigest string `gorm:"type:varchar(64);not null"`
	ExpiresAt     int64  `gorm:"not null;index"`
	CreatedAt     int64  `gorm:"not null"`
}

func (AuthScopeBindingRecord) TableName() string { return "cmh_auth_scope_bindings" }

// OEMSiteDomainRecord is the one custom domain of a site. Host is what the
// owner asked for; VerifiedHost is set only after the domain's CNAME matched
// the site's challenge target, and its unique index lets one site at most
// serve a host. Changing Host clears VerifiedHost and issues a new Challenge.
type OEMSiteDomainRecord struct {
	SiteID       string  `gorm:"type:varchar(32);primaryKey"`
	OwnerUserID  int     `gorm:"not null"`
	Host         string  `gorm:"type:varchar(253);not null"`
	Challenge    string  `gorm:"type:varchar(32);not null"`
	VerifiedHost *string `gorm:"type:varchar(253);uniqueIndex"`
	LastError    string  `gorm:"type:varchar(32);not null"`
	CheckedAt    int64   `gorm:"not null;index"`
	VerifiedAt   int64   `gorm:"not null"`
	CreatedAt    int64   `gorm:"not null"`
	UpdatedAt    int64   `gorm:"not null"`
}

func (OEMSiteDomainRecord) TableName() string { return "cmh_oem_site_domains" }

// OEMSiteAppearanceRecord is the storefront look and search settings of a
// site. A site without a row uses the default theme, no SEO overrides and
// allows indexing.
type OEMSiteAppearanceRecord struct {
	SiteID         string `gorm:"type:varchar(32);primaryKey"`
	OwnerUserID    int    `gorm:"not null"`
	Theme          string `gorm:"type:varchar(16);not null"`
	SEOTitle       string `gorm:"type:varchar(500);not null"`
	SEODescription string `gorm:"type:varchar(1200);not null"`
	SEOKeywords    string `gorm:"type:varchar(800);not null"`
	NoIndex        bool   `gorm:"not null"`
	Version        int64  `gorm:"not null"`
	UpdatedAt      int64  `gorm:"not null"`
}

func (OEMSiteAppearanceRecord) TableName() string { return "cmh_oem_site_appearances" }

// OEMSiteLogoRecord holds the uploaded logo bytes apart from the settings, so
// page renders never load it. Data is a fully decoded PNG, JPEG or WebP of at
// most 1 MiB and 2048x2048 pixels.
type OEMSiteLogoRecord struct {
	SiteID      string `gorm:"type:varchar(32);primaryKey"`
	OwnerUserID int    `gorm:"not null"`
	ContentType string `gorm:"type:varchar(32);not null"`
	Data        []byte `gorm:"not null"`
	UpdatedAt   int64  `gorm:"not null"`
}

func (OEMSiteLogoRecord) TableName() string { return "cmh_oem_site_logos" }

// MigrateOEM is explicit: it is not registered as H6, run on requests, or
// invoked by importing the extension. It never migrates official tables.
func MigrateOEM(db *gorm.DB) error {
	if db == nil {
		return errors.New("OEM database unavailable")
	}
	return db.AutoMigrate(&OEMSiteRecord{}, &OEMSiteAuditRecord{}, &OEMCustomerInvitationRecord{}, &OEMCustomerRecord{}, &OEMCustomerAuditRecord{}, &OEMCustomerNoteRecord{}, &OEMTeamInvitationRecord{}, &OEMTeamMemberRecord{}, &OEMTeamAuditRecord{}, &AuthScopeBindingRecord{}, &OEMSiteDomainRecord{}, &OEMSiteAppearanceRecord{}, &OEMSiteLogoRecord{})
}
