package brand

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/ext/migrations"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CustomerInvitationTTL is deliberately finite and cannot be supplied by callers.
const CustomerInvitationTTL = 24 * time.Hour

type Customer struct {
	UserID    int    `json:"user_id"`
	SiteID    string `json:"site_id"`
	Active    bool   `json:"active"`
	JoinedAt  int64  `json:"joined_at"`
	RevokedAt int64  `json:"revoked_at"`
}
type CustomerNote struct {
	Note      string `json:"note"`
	Version   int64  `json:"version"`
	UpdatedAt int64  `json:"updated_at"`
}

// CustomerNote requires an exact owner/site relationship, including a revoked
// historical relationship. It grants no membership or gateway authority.
func (s *SiteStore) CustomerNote(ctx context.Context, owner int, siteID string, userID int) (CustomerNote, error) {
	if _, err := s.Get(ctx, owner, siteID); err != nil {
		return CustomerNote{}, err
	}
	var customer migrations.OEMCustomerRecord
	if err := s.db.WithContext(ctx).Where("user_id = ? AND site_id = ? AND owner_user_id = ?", userID, siteID, owner).First(&customer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return CustomerNote{}, ErrOEMNotFound
		}
		return CustomerNote{}, err
	}
	var record migrations.OEMCustomerNoteRecord
	err := s.db.WithContext(ctx).Where("user_id = ? AND site_id = ? AND owner_user_id = ?", userID, siteID, owner).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CustomerNote{}, nil
	}
	if err != nil {
		return CustomerNote{}, err
	}
	return CustomerNote{Note: record.Note, Version: record.Version, UpdatedAt: record.UpdatedAt}, nil
}

func (s *SiteStore) UpdateCustomerNote(ctx context.Context, owner int, siteID string, userID int, note string, version int64) (CustomerNote, error) {
	if _, err := s.Get(ctx, owner, siteID); err != nil {
		return CustomerNote{}, err
	}
	if userID <= 0 || version < 0 || version > (1<<53)-2 || !utf8.ValidString(note) || len(note) > 2000 {
		return CustomerNote{}, ErrOEMInvalid
	}
	for _, r := range note {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return CustomerNote{}, ErrOEMInvalid
		}
	}
	auditID, err := newOEMID()
	if err != nil {
		return CustomerNote{}, err
	}
	now := time.Now().Unix()
	var committed migrations.OEMCustomerNoteRecord
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var customer migrations.OEMCustomerRecord
		if err := tx.Where("user_id = ? AND site_id = ? AND owner_user_id = ?", userID, siteID, owner).First(&customer).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOEMNotFound
			}
			return err
		}
		if version == 0 {
			// Reserve the key without relying on MySQL's DoNothing RowsAffected.
			// Every writer then performs the same version-zero CAS.
			seed := migrations.OEMCustomerNoteRecord{UserID: userID, SiteID: siteID, OwnerUserID: owner}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
				return err
			}
		}
		changed := tx.Model(&migrations.OEMCustomerNoteRecord{}).Where("user_id = ? AND site_id = ? AND owner_user_id = ? AND version = ?", userID, siteID, owner, version).Updates(map[string]any{"note": note, "version": version + 1, "updated_at": now})
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return ErrOEMConflict
		}
		if err := tx.Create(&migrations.OEMCustomerAuditRecord{ID: auditID, SiteID: siteID, UserID: userID, ActorUserID: owner, Action: "customer.note_updated", CreatedAt: now}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ? AND site_id = ? AND owner_user_id = ?", userID, siteID, owner).First(&committed).Error
	})
	if err != nil {
		return CustomerNote{}, err
	}
	return CustomerNote{Note: committed.Note, Version: committed.Version, UpdatedAt: committed.UpdatedAt}, nil
}

type CustomerInvitation struct {
	ID           string `json:"id"`
	SiteID       string `json:"site_id"`
	TargetUserID int    `json:"target_user_id"`
	ExpiresAt    int64  `json:"expires_at"`
	CreatedAt    int64  `json:"created_at"`
	AcceptedAt   int64  `json:"accepted_at"`
}
type CreatedCustomerInvitation struct {
	Invitation CustomerInvitation `json:"invitation"`
	Token      string             `json:"token"`
}

func customerView(r migrations.OEMCustomerRecord) Customer {
	return Customer{UserID: r.UserID, SiteID: r.SiteID, Active: r.Active, JoinedAt: r.JoinedAt, RevokedAt: r.RevokedAt}
}
func invitationView(r migrations.OEMCustomerInvitationRecord) CustomerInvitation {
	return CustomerInvitation{ID: r.ID, SiteID: r.SiteID, TargetUserID: r.TargetUserID, ExpiresAt: r.ExpiresAt, CreatedAt: r.CreatedAt, AcceptedAt: r.AcceptedAt}
}

type CustomerAudit struct {
	ID        string `json:"id"`
	Action    string `json:"action"`
	UserID    int    `json:"user_id"`
	CreatedAt int64  `json:"created_at"`
}

func (s *SiteStore) CustomerAudits(ctx context.Context, owner int, siteID string, page Page) ([]CustomerAudit, error) {
	if _, err := s.Get(ctx, owner, siteID); err != nil {
		return nil, err
	}
	var records []migrations.OEMCustomerAuditRecord
	if err := page.window(s.db.WithContext(ctx).Where("site_id = ?", siteID).Order("created_at DESC, id DESC")).Find(&records).Error; err != nil {
		return nil, err
	}
	out := make([]CustomerAudit, 0, len(records))
	for _, r := range records {
		out = append(out, CustomerAudit{ID: r.ID, Action: r.Action, UserID: r.UserID, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// Existing ordinary users only. No official user creation, financial mutations or
// username/email lookup is performed. All ineligible targets share one error.
func ordinaryCustomer(id int) bool {
	if id <= 0 {
		return false
	}
	user, err := model.GetUserById(id, false)
	return err == nil && user != nil && user.Role == common.RoleCommonUser && user.Status == common.UserStatusEnabled
}
func (s *SiteStore) Customers(ctx context.Context, owner int, siteID string, page Page) ([]Customer, error) {
	if _, err := s.Get(ctx, owner, siteID); err != nil {
		return nil, err
	}
	var rows []migrations.OEMCustomerRecord
	if err := page.window(s.db.WithContext(ctx).Where("site_id = ? AND owner_user_id = ?", siteID, owner).Order("joined_at DESC, user_id DESC")).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Customer, 0, len(rows))
	for _, r := range rows {
		out = append(out, customerView(r))
	}
	return out, nil
}
func (s *SiteStore) CustomerInvitations(ctx context.Context, owner int, siteID string) ([]CustomerInvitation, error) {
	if _, err := s.Get(ctx, owner, siteID); err != nil {
		return nil, err
	}
	var rows []migrations.OEMCustomerInvitationRecord
	if err := s.db.WithContext(ctx).Where("site_id = ? AND owner_user_id = ?", siteID, owner).Order("created_at DESC, id DESC").Limit(100).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]CustomerInvitation, 0, len(rows))
	for _, r := range rows {
		out = append(out, invitationView(r))
	}
	return out, nil
}
func (s *SiteStore) InviteCustomer(ctx context.Context, owner int, siteID string, target int) (CreatedCustomerInvitation, error) {
	if _, err := s.Get(ctx, owner, siteID); err != nil {
		return CreatedCustomerInvitation{}, err
	}
	if target == owner || !ordinaryCustomer(target) {
		return CreatedCustomerInvitation{}, ErrOEMInvalid
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return CreatedCustomerInvitation{}, err
	}
	token := hex.EncodeToString(raw[:])
	digest := sha256.Sum256([]byte(token))
	id, err := newOEMID()
	if err != nil {
		return CreatedCustomerInvitation{}, err
	}
	auditID, err := newOEMID()
	if err != nil {
		return CreatedCustomerInvitation{}, err
	}
	now := time.Now().Unix()
	record := migrations.OEMCustomerInvitationRecord{ID: id, SiteID: siteID, OwnerUserID: owner, TargetUserID: target, TokenHash: hex.EncodeToString(digest[:]), CreatedAt: now, ExpiresAt: now + int64(CustomerInvitationTTL/time.Second)}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var site migrations.OEMSiteRecord
		if err := tx.Where("id = ? AND owner_user_id = ?", siteID, owner).First(&site).Error; err != nil {
			return ErrOEMNotFound
		}
		var count int64
		if err := tx.Model(&migrations.OEMCustomerRecord{}).Where("user_id = ?", target).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrOEMConflict
		}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		return tx.Create(&migrations.OEMCustomerAuditRecord{ID: auditID, SiteID: siteID, UserID: target, ActorUserID: owner, Action: "customer.invited", CreatedAt: now}).Error
	})
	if err != nil {
		return CreatedCustomerInvitation{}, err
	}
	return CreatedCustomerInvitation{Invitation: invitationView(record), Token: token}, nil
}
func (s *SiteStore) CustomerMembership(ctx context.Context, userID int) (*Customer, error) {
	if s == nil || s.db == nil {
		return nil, ErrOEMUnavailable
	}
	if userID <= 0 {
		return nil, ErrOEMInvalid
	}
	var row migrations.OEMCustomerRecord
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	value := customerView(row)
	return &value, nil
}
func (s *SiteStore) AcceptCustomerInvitation(ctx context.Context, userID int, token string) (Customer, error) {
	if s == nil || s.db == nil {
		return Customer{}, ErrOEMUnavailable
	}
	// Fixed-size hex token; avoid accepting alternate representations.
	decoded, err := hex.DecodeString(token)
	if err != nil || len(decoded) != 32 || len(token) != 64 || hex.EncodeToString(decoded) != token || !ordinaryCustomer(userID) {
		return Customer{}, ErrOEMInvalid
	}
	digest := sha256.Sum256([]byte(token))
	now := time.Now().Unix()
	auditID, err := newOEMID()
	if err != nil {
		return Customer{}, err
	}
	var committed migrations.OEMCustomerRecord
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var invite migrations.OEMCustomerInvitationRecord
		if err := tx.Where("token_hash = ? AND target_user_id = ?", hex.EncodeToString(digest[:]), userID).First(&invite).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOEMNotFound
			}
			return err
		}
		var site migrations.OEMSiteRecord
		if err := tx.Where("id = ? AND owner_user_id = ?", invite.SiteID, invite.OwnerUserID).First(&site).Error; err != nil {
			return ErrOEMNotFound
		}
		if invite.AcceptedAt > 0 {
			if err := tx.Where("user_id = ? AND site_id = ? AND owner_user_id = ? AND invitation_id = ? AND active = ?", userID, invite.SiteID, invite.OwnerUserID, invite.ID, true).First(&committed).Error; err != nil {
				return ErrOEMConflict
			}
			return nil
		}
		if invite.ExpiresAt <= now {
			return ErrOEMNotFound
		}
		changed := tx.Model(&migrations.OEMCustomerInvitationRecord{}).Where("id = ? AND accepted_at = ? AND expires_at > ?", invite.ID, 0, now).Update("accepted_at", now)
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return ErrOEMConflict
		}
		record := migrations.OEMCustomerRecord{UserID: userID, SiteID: invite.SiteID, OwnerUserID: invite.OwnerUserID, InvitationID: invite.ID, Active: true, JoinedAt: now}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error; err != nil {
			return err
		}
		var stored migrations.OEMCustomerRecord
		if err := tx.Where("user_id = ?", userID).First(&stored).Error; err != nil {
			return err
		}
		if stored.InvitationID != invite.ID || stored.SiteID != invite.SiteID || stored.OwnerUserID != invite.OwnerUserID || !stored.Active {
			return ErrOEMConflict
		}
		if err := tx.Create(&migrations.OEMCustomerAuditRecord{ID: auditID, SiteID: invite.SiteID, UserID: userID, ActorUserID: userID, Action: "customer.accepted", CreatedAt: now}).Error; err != nil {
			return err
		}
		committed = record
		return nil
	})
	if err != nil {
		return Customer{}, err
	}
	return customerView(committed), nil
}
func (s *SiteStore) RevokeCustomer(ctx context.Context, owner int, siteID string, userID int) (Customer, error) {
	if _, err := s.Get(ctx, owner, siteID); err != nil {
		return Customer{}, err
	}
	if userID <= 0 {
		return Customer{}, ErrOEMInvalid
	}
	now := time.Now().Unix()
	auditID, err := newOEMID()
	if err != nil {
		return Customer{}, err
	}
	var record migrations.OEMCustomerRecord
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		changed := tx.Model(&migrations.OEMCustomerRecord{}).Where("user_id = ? AND site_id = ? AND owner_user_id = ? AND active = ?", userID, siteID, owner, true).Updates(map[string]any{"active": false, "revoked_at": now})
		if changed.Error != nil {
			return changed.Error
		}
		if err := tx.Where("user_id = ? AND site_id = ? AND owner_user_id = ?", userID, siteID, owner).First(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOEMNotFound
			}
			return err
		}
		if changed.RowsAffected == 0 {
			return nil
		}
		return tx.Create(&migrations.OEMCustomerAuditRecord{ID: auditID, SiteID: siteID, UserID: userID, ActorUserID: owner, Action: "customer.revoked", CreatedAt: now}).Error
	})
	if err != nil {
		return Customer{}, err
	}
	return customerView(record), nil
}
