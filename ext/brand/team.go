package brand

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/ext/migrations"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Team membership is an additional read predicate, never an official role.
type TeamMember struct {
	UserID    int    `json:"user_id"`
	SiteID    string `json:"site_id"`
	Active    bool   `json:"active"`
	Version   int64  `json:"version"`
	JoinedAt  int64  `json:"joined_at"`
	RevokedAt int64  `json:"revoked_at"`
}
type TeamInvitation = CustomerInvitation
type CreatedTeamInvitation struct {
	Invitation TeamInvitation `json:"invitation"`
	Token      string         `json:"token"`
}
type TeamAudit = CustomerAudit
type ReadableSite struct {
	Site
	Access string `json:"access"`
}

func teamMemberView(r migrations.OEMTeamMemberRecord) TeamMember {
	return TeamMember{UserID: r.UserID, SiteID: r.SiteID, Active: r.Active, Version: r.Version, JoinedAt: r.JoinedAt, RevokedAt: r.RevokedAt}
}
func teamInvitationView(r migrations.OEMTeamInvitationRecord) TeamInvitation {
	return TeamInvitation{ID: r.ID, SiteID: r.SiteID, TargetUserID: r.TargetUserID, ExpiresAt: r.ExpiresAt, CreatedAt: r.CreatedAt, AcceptedAt: r.AcceptedAt}
}
func eligibleTeamReader(id int) bool {
	if id <= 0 {
		return false
	}
	user, err := model.GetUserById(id, false)
	return err == nil && user != nil && user.Status == common.UserStatusEnabled && user.Role == common.RoleAdminUser && authz.Can(id, user.Role, OEMRead)
}

// ReadSiteOwner returns an independently authorized data owner. The caller's
// authenticated identity is never replaced. Revocation takes effect next request;
// this does not serialize an already-authorized read against a concurrent revoke.
func (s *SiteStore) ReadSiteOwner(ctx context.Context, actor int, siteID string) (int, string, error) {
	if s == nil || s.db == nil {
		return 0, "", ErrOEMUnavailable
	}
	if actor <= 0 || len(siteID) != 32 {
		return 0, "", ErrOEMInvalid
	}
	var site migrations.OEMSiteRecord
	if err := s.db.WithContext(ctx).Where("id = ?", siteID).First(&site).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, "", ErrOEMNotFound
		}
		return 0, "", err
	}
	// Preserve owner authorization through the existing HTTP permission middleware.
	if site.OwnerUserID == actor {
		return actor, "owner", nil
	}
	if !eligibleTeamReader(actor) {
		return 0, "", ErrOEMNotFound
	}
	var count int64
	err := s.db.WithContext(ctx).Model(&migrations.OEMTeamMemberRecord{}).Where("site_id = ? AND user_id = ? AND owner_user_id = ? AND active = ?", siteID, actor, site.OwnerUserID, true).Count(&count).Error
	if err != nil {
		return 0, "", err
	}
	if count != 1 {
		return 0, "", ErrOEMNotFound
	}
	return site.OwnerUserID, "readonly", nil
}
func (s *SiteStore) ReadableSites(ctx context.Context, actor int) ([]ReadableSite, error) {
	if s == nil || s.db == nil {
		return nil, ErrOEMUnavailable
	}
	if actor <= 0 {
		return nil, ErrOEMInvalid
	}
	var rows []migrations.OEMSiteRecord
	query := s.db.WithContext(ctx).Where("owner_user_id = ?", actor)
	if eligibleTeamReader(actor) {
		members := s.db.Model(&migrations.OEMTeamMemberRecord{}).Select("site_id").Where("user_id = ? AND active = ?", actor, true)
		query = s.db.WithContext(ctx).Where("owner_user_id = ? OR id IN (?)", actor, members)
	}
	if err := query.Order("created_at DESC, id DESC").Limit(100).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ReadableSite, 0, len(rows))
	for _, row := range rows {
		_, access, err := s.ReadSiteOwner(ctx, actor, row.ID)
		if errors.Is(err, ErrOEMNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, ReadableSite{Site: siteView(row), Access: access})
	}
	return out, nil
}
func (s *SiteStore) TeamMembers(ctx context.Context, owner int, siteID string) ([]TeamMember, error) {
	if _, err := s.Get(ctx, owner, siteID); err != nil {
		return nil, err
	}
	var rows []migrations.OEMTeamMemberRecord
	if err := s.db.WithContext(ctx).Where("site_id = ? AND owner_user_id = ?", siteID, owner).Order("user_id ASC").Limit(100).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]TeamMember, 0, len(rows))
	for _, r := range rows {
		out = append(out, teamMemberView(r))
	}
	return out, nil
}
func (s *SiteStore) TeamInvitations(ctx context.Context, owner int, siteID string) ([]TeamInvitation, error) {
	if _, err := s.Get(ctx, owner, siteID); err != nil {
		return nil, err
	}
	var rows []migrations.OEMTeamInvitationRecord
	if err := s.db.WithContext(ctx).Where("site_id = ? AND owner_user_id = ?", siteID, owner).Order("created_at DESC, id DESC").Limit(100).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]TeamInvitation, 0, len(rows))
	for _, r := range rows {
		out = append(out, teamInvitationView(r))
	}
	return out, nil
}
func (s *SiteStore) TeamAudits(ctx context.Context, owner int, siteID string) ([]TeamAudit, error) {
	if _, err := s.Get(ctx, owner, siteID); err != nil {
		return nil, err
	}
	var rows []migrations.OEMTeamAuditRecord
	if err := s.db.WithContext(ctx).Where("site_id = ?", siteID).Order("created_at DESC, id DESC").Limit(100).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]TeamAudit, 0, len(rows))
	for _, r := range rows {
		out = append(out, TeamAudit{ID: r.ID, Action: r.Action, UserID: r.UserID, CreatedAt: r.CreatedAt})
	}
	return out, nil
}
func (s *SiteStore) InviteTeamMember(ctx context.Context, owner int, siteID string, target int) (CreatedTeamInvitation, error) {
	if _, err := s.Get(ctx, owner, siteID); err != nil {
		return CreatedTeamInvitation{}, err
	}
	if target == owner || !eligibleTeamReader(target) {
		return CreatedTeamInvitation{}, ErrOEMInvalid
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return CreatedTeamInvitation{}, err
	}
	token := hex.EncodeToString(raw[:])
	digest := sha256.Sum256([]byte(token))
	id, err := newOEMID()
	if err != nil {
		return CreatedTeamInvitation{}, err
	}
	auditID, err := newOEMID()
	if err != nil {
		return CreatedTeamInvitation{}, err
	}
	now := time.Now().Unix()
	row := migrations.OEMTeamInvitationRecord{ID: id, SiteID: siteID, OwnerUserID: owner, TargetUserID: target, TokenHash: hex.EncodeToString(digest[:]), CreatedAt: now, ExpiresAt: now + int64(CustomerInvitationTTL/time.Second)}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var site migrations.OEMSiteRecord
		if err := tx.Where("id = ? AND owner_user_id = ?", siteID, owner).First(&site).Error; err != nil {
			return ErrOEMNotFound
		}
		var current migrations.OEMTeamMemberRecord
		lookup := tx.Where("site_id = ? AND user_id = ?", siteID, target).First(&current).Error
		if lookup != nil && !errors.Is(lookup, gorm.ErrRecordNotFound) {
			return lookup
		}
		if lookup == nil {
			if current.OwnerUserID != owner || current.Active {
				return ErrOEMConflict
			}
			row.ExpectedMemberVersion = current.Version
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return tx.Create(&migrations.OEMTeamAuditRecord{ID: auditID, SiteID: siteID, UserID: target, ActorUserID: owner, Action: "team.invited", CreatedAt: now}).Error
	})
	if err != nil {
		return CreatedTeamInvitation{}, err
	}
	return CreatedTeamInvitation{Invitation: teamInvitationView(row), Token: token}, nil
}
func (s *SiteStore) AcceptTeamInvitation(ctx context.Context, actor int, token string) (TeamMember, error) {
	if s == nil || s.db == nil {
		return TeamMember{}, ErrOEMUnavailable
	}
	decoded, err := hex.DecodeString(token)
	if err != nil || len(decoded) != 32 || len(token) != 64 || hex.EncodeToString(decoded) != token || !eligibleTeamReader(actor) {
		return TeamMember{}, ErrOEMInvalid
	}
	digest := sha256.Sum256([]byte(token))
	now := time.Now().Unix()
	auditID, err := newOEMID()
	if err != nil {
		return TeamMember{}, err
	}
	var committed migrations.OEMTeamMemberRecord
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var invite migrations.OEMTeamInvitationRecord
		if err := tx.Where("token_hash = ? AND target_user_id = ?", hex.EncodeToString(digest[:]), actor).First(&invite).Error; err != nil {
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
			if err := tx.Where("site_id = ? AND user_id = ? AND owner_user_id = ? AND invitation_id = ? AND active = ?", invite.SiteID, actor, invite.OwnerUserID, invite.ID, true).First(&committed).Error; err != nil {
				return ErrOEMConflict
			}
			return nil
		}
		if invite.ExpiresAt <= now {
			return ErrOEMNotFound
		}
		changed := tx.Model(&migrations.OEMTeamInvitationRecord{}).Where("id = ? AND accepted_at = ? AND expires_at > ?", invite.ID, 0, now).Update("accepted_at", now)
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return ErrOEMConflict
		}
		seed := migrations.OEMTeamMemberRecord{SiteID: invite.SiteID, UserID: actor, OwnerUserID: invite.OwnerUserID}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
			return err
		}
		var current migrations.OEMTeamMemberRecord
		if err := tx.Where("site_id = ? AND user_id = ?", invite.SiteID, actor).First(&current).Error; err != nil {
			return err
		}
		if current.OwnerUserID != invite.OwnerUserID || current.Active || current.Version != invite.ExpectedMemberVersion || current.Version >= (1<<53)-1 {
			return ErrOEMConflict
		}
		changed = tx.Model(&migrations.OEMTeamMemberRecord{}).Where("site_id = ? AND user_id = ? AND owner_user_id = ? AND active = ? AND version = ?", invite.SiteID, actor, invite.OwnerUserID, false, current.Version).Updates(map[string]any{"active": true, "version": current.Version + 1, "invitation_id": invite.ID, "joined_at": now, "revoked_at": 0})
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return ErrOEMConflict
		}
		if err := tx.Create(&migrations.OEMTeamAuditRecord{ID: auditID, SiteID: invite.SiteID, UserID: actor, ActorUserID: actor, Action: "team.accepted", CreatedAt: now}).Error; err != nil {
			return err
		}
		return tx.Where("site_id = ? AND user_id = ?", invite.SiteID, actor).First(&committed).Error
	})
	if err != nil {
		return TeamMember{}, err
	}
	return teamMemberView(committed), nil
}
func (s *SiteStore) RevokeTeamMember(ctx context.Context, owner int, siteID string, target int, version int64) (TeamMember, error) {
	if _, err := s.Get(ctx, owner, siteID); err != nil {
		return TeamMember{}, err
	}
	if target <= 0 || version < 1 || version >= (1<<53)-1 {
		return TeamMember{}, ErrOEMInvalid
	}
	id, err := newOEMID()
	if err != nil {
		return TeamMember{}, err
	}
	now := time.Now().Unix()
	var committed migrations.OEMTeamMemberRecord
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		changed := tx.Model(&migrations.OEMTeamMemberRecord{}).Where("site_id = ? AND user_id = ? AND owner_user_id = ? AND active = ? AND version = ?", siteID, target, owner, true, version).Updates(map[string]any{"active": false, "version": version + 1, "revoked_at": now})
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return ErrOEMConflict
		}
		if err := tx.Create(&migrations.OEMTeamAuditRecord{ID: id, SiteID: siteID, UserID: target, ActorUserID: owner, Action: "team.revoked", CreatedAt: now}).Error; err != nil {
			return err
		}
		return tx.Where("site_id = ? AND user_id = ?", siteID, target).First(&committed).Error
	})
	if err != nil {
		return TeamMember{}, err
	}
	return teamMemberView(committed), nil
}
