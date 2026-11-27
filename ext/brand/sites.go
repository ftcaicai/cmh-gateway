package brand

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/ext/migrations"
	"gorm.io/gorm"
)

var (
	ErrOEMInvalid     = errors.New("invalid OEM input")
	ErrOEMNotFound    = errors.New("OEM site not found")
	ErrOEMConflict    = errors.New("OEM site changed; reload before saving")
	ErrOEMUnavailable = errors.New("OEM database unavailable")
)

// SiteStore owns nonfinancial brand profiles, never official user or wallet rows.
// Every operation scopes by authenticated owner, even after HTTP authorization.
type SiteStore struct{ db *gorm.DB }

func NewSiteStore(db *gorm.DB) *SiteStore { return &SiteStore{db: db} }

// Page is an offset window over a list ordered newest first. The zero value
// is the first page of the default size; Limit is capped at pageMaxLimit.
type Page struct {
	Offset int
	Limit  int
}

const pageMaxLimit = 100

func (p Page) window(db *gorm.DB) *gorm.DB {
	limit := p.Limit
	if limit <= 0 || limit > pageMaxLimit {
		limit = pageMaxLimit
	}
	return db.Offset(max(p.Offset, 0)).Limit(limit)
}

type SiteInput struct {
	Name         string `json:"name"`
	SupportEmail string `json:"support_email"`
	Description  string `json:"description"`
}

type Site struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	SupportEmail string `json:"support_email"`
	Description  string `json:"description"`
	Version      int64  `json:"version"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

func siteView(r migrations.OEMSiteRecord) Site {
	return Site{ID: r.ID, Name: r.Name, SupportEmail: r.SupportEmail, Description: r.Description, Version: r.Version, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func validateSiteInput(input SiteInput) bool {
	for _, f := range []struct {
		value string
		max   int
	}{{input.Name, 100}, {input.SupportEmail, 254}, {input.Description, 1000}} {
		if !utf8.ValidString(f.value) || len(f.value) > f.max {
			return false
		}
		for _, r := range f.value {
			if unicode.IsControl(r) {
				return false
			}
		}
	}
	if strings.TrimSpace(input.Name) == "" || input.Name != strings.TrimSpace(input.Name) {
		return false
	}
	if input.SupportEmail != "" {
		address, err := mail.ParseAddress(input.SupportEmail)
		if err != nil || address.Address != input.SupportEmail || address.Name != "" {
			return false
		}
	}
	return true
}

func newOEMID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func (s *SiteStore) List(ctx context.Context, owner int) ([]Site, error) {
	if s == nil || s.db == nil {
		return nil, ErrOEMUnavailable
	}
	if owner <= 0 {
		return nil, ErrOEMInvalid
	}
	var records []migrations.OEMSiteRecord
	if err := s.db.WithContext(ctx).Where("owner_user_id = ?", owner).Order("created_at DESC, id DESC").Limit(100).Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]Site, 0, len(records))
	for _, r := range records {
		result = append(result, siteView(r))
	}
	return result, nil
}

func (s *SiteStore) Get(ctx context.Context, owner int, id string) (Site, error) {
	if s == nil || s.db == nil {
		return Site{}, ErrOEMUnavailable
	}
	if owner <= 0 || len(id) != 32 {
		return Site{}, ErrOEMInvalid
	}
	var record migrations.OEMSiteRecord
	err := s.db.WithContext(ctx).Where("id = ? AND owner_user_id = ?", id, owner).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Site{}, ErrOEMNotFound
	}
	return siteView(record), err
}

func (s *SiteStore) Create(ctx context.Context, owner int, input SiteInput) (Site, error) {
	if s == nil || s.db == nil {
		return Site{}, ErrOEMUnavailable
	}
	if owner <= 0 || !validateSiteInput(input) {
		return Site{}, ErrOEMInvalid
	}
	id, err := newOEMID()
	if err != nil {
		return Site{}, err
	}
	auditID, err := newOEMID()
	if err != nil {
		return Site{}, err
	}
	now := time.Now().Unix()
	record := migrations.OEMSiteRecord{ID: id, OwnerUserID: owner, Name: input.Name, SupportEmail: input.SupportEmail, Description: input.Description, Version: 1, CreatedAt: now, UpdatedAt: now}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		return tx.Create(&migrations.OEMSiteAuditRecord{ID: auditID, SiteID: id, ActorUserID: owner, Action: "site.created", Version: 1, CreatedAt: now}).Error
	})
	if err != nil {
		return Site{}, err
	}
	return siteView(record), nil
}

type SiteAudit struct {
	ID        string `json:"id"`
	Action    string `json:"action"`
	Version   int64  `json:"version"`
	CreatedAt int64  `json:"created_at"`
}

func (s *SiteStore) Audits(ctx context.Context, owner int, id string, page Page) ([]SiteAudit, error) {
	if _, err := s.Get(ctx, owner, id); err != nil {
		return nil, err
	}
	var records []migrations.OEMSiteAuditRecord
	if err := page.window(s.db.WithContext(ctx).Where("site_id = ?", id).Order("created_at DESC, version DESC, id DESC")).Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]SiteAudit, 0, len(records))
	for _, record := range records {
		result = append(result, SiteAudit{ID: record.ID, Action: record.Action, Version: record.Version, CreatedAt: record.CreatedAt})
	}
	return result, nil
}

func (s *SiteStore) Update(ctx context.Context, owner int, id string, version int64, input SiteInput) (Site, error) {
	if s == nil || s.db == nil {
		return Site{}, ErrOEMUnavailable
	}
	if owner <= 0 || len(id) != 32 || version < 1 || version >= (1<<53)-1 || !validateSiteInput(input) {
		return Site{}, ErrOEMInvalid
	}
	auditID, err := newOEMID()
	if err != nil {
		return Site{}, err
	}
	now := time.Now().Unix()
	var committed migrations.OEMSiteRecord
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&migrations.OEMSiteRecord{}).Where("id = ? AND owner_user_id = ? AND version = ?", id, owner, version).Updates(map[string]any{"name": input.Name, "support_email": input.SupportEmail, "description": input.Description, "version": version + 1, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			var count int64
			if err := tx.Model(&migrations.OEMSiteRecord{}).Where("id = ? AND owner_user_id = ?", id, owner).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return ErrOEMNotFound
			}
			return ErrOEMConflict
		}
		if err := tx.Create(&migrations.OEMSiteAuditRecord{ID: auditID, SiteID: id, ActorUserID: owner, Action: "site.updated", Version: version + 1, CreatedAt: now}).Error; err != nil {
			return err
		}
		return tx.Where("id = ? AND owner_user_id = ?", id, owner).First(&committed).Error
	})
	if err != nil {
		return Site{}, err
	}
	return siteView(committed), nil
}
