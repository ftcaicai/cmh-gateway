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

// MigrateOEM is explicit: it is not registered as H6, run on requests, or
// invoked by importing the extension. It never migrates official tables.
func MigrateOEM(db *gorm.DB) error {
	if db == nil {
		return errors.New("OEM database unavailable")
	}
	return db.AutoMigrate(&OEMSiteRecord{}, &OEMSiteAuditRecord{}, &OEMCustomerInvitationRecord{}, &OEMCustomerRecord{}, &OEMCustomerAuditRecord{})
}
