package brand

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/ext/migrations"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The owner can take a storefront offline without removing its domain or
// customers: every page and API on the site's hosts answers 403 "站点已暂停
// 服务" until it is resumed. Sessions are not revoked; they simply cannot be
// used on a paused host. Certificates keep renewing, so resuming is instant.

// SitePause is the owner's view of the switch.
type SitePause struct {
	Paused   bool  `json:"paused"`
	PausedAt int64 `json:"paused_at"`
}

// Pause reads the switch for the owner.
func (s *SiteStore) Pause(ctx context.Context, owner int, siteID string) (SitePause, error) {
	if _, err := s.Get(ctx, owner, siteID); err != nil {
		return SitePause{}, err
	}
	var record migrations.OEMSitePauseRecord
	err := s.db.WithContext(ctx).Where("site_id = ?", siteID).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return SitePause{}, nil
	}
	return SitePause{Paused: err == nil, PausedAt: record.PausedAt}, err
}

// SetPaused takes the storefront offline or brings it back. Repeating the
// current state changes nothing and writes no audit.
func (s *SiteStore) SetPaused(ctx context.Context, owner int, siteID string, paused bool) (SitePause, error) {
	site, err := s.Get(ctx, owner, siteID)
	if err != nil {
		return SitePause{}, err
	}
	auditID, err := newOEMID()
	if err != nil {
		return SitePause{}, err
	}
	now := time.Now().Unix()
	changed := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		action := "site.resumed"
		if paused {
			action = "site.paused"
			created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&migrations.OEMSitePauseRecord{SiteID: siteID, OwnerUserID: owner, PausedAt: now})
			if created.Error != nil {
				return created.Error
			}
			changed = created.RowsAffected == 1
		} else {
			removed := tx.Where("site_id = ?", siteID).Delete(&migrations.OEMSitePauseRecord{})
			if removed.Error != nil {
				return removed.Error
			}
			changed = removed.RowsAffected == 1
		}
		if !changed {
			return nil
		}
		return tx.Create(&migrations.OEMSiteAuditRecord{ID: auditID, SiteID: siteID, ActorUserID: owner, Action: action, Version: site.Version, CreatedAt: now}).Error
	})
	if err != nil {
		return SitePause{}, err
	}
	if changed {
		publishBrandHosts(ctx)
	}
	return s.Pause(ctx, owner, siteID)
}

// pausedSites is the set of paused site IDs, loaded and replaced together
// with the verified host map so a pause reaches every node through the same
// sync topic. The resolver never queries the database for it.
var pausedSites atomic.Pointer[map[string]bool]

func sitePaused(siteID string) bool {
	verifiedHostsOnce.Do(loadBrandHostsOnStart)
	ids := pausedSites.Load()
	return ids != nil && (*ids)[siteID]
}

func loadPausedSites(ctx context.Context, db *gorm.DB) error {
	var records []migrations.OEMSitePauseRecord
	if err := db.WithContext(ctx).Select("site_id").Find(&records).Error; err != nil {
		return err
	}
	ids := make(map[string]bool, len(records))
	for _, record := range records {
		ids[record.SiteID] = true
	}
	pausedSites.Store(&ids)
	return nil
}
