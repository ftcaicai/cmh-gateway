package brand

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	_ "image/jpeg" // registers the JPEG decoder for image.Decode
	_ "image/png"  // registers the PNG decoder for image.Decode
	"io"
	"net/http"
	"slices"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/ext/migrations"
	"github.com/gin-gonic/gin"
	_ "golang.org/x/image/webp" // registers the WebP decoder for image.Decode
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Storefront appearance (T2.4): theme, logo and basic SEO of a brand site.
// Owner only. Everything here is presentation; it changes no account, price or
// permission.

// Themes keep the old system's identifiers: cloud-white business, dark
// technology and warm-white magazine.
var brandThemes = []string{"default", "technology", "business"}

const (
	logoMaxBytes  = 1 << 20
	logoMaxPixels = 2048
)

// SiteAppearance is the owner's view; the storefront reads the same values.
type SiteAppearance struct {
	Theme          string `json:"theme"`
	SEOTitle       string `json:"seo_title"`
	SEODescription string `json:"seo_description"`
	SEOKeywords    string `json:"seo_keywords"`
	// AllowIndexing false tells search engines to stay away from every page.
	AllowIndexing bool  `json:"allow_indexing"`
	HasLogo       bool  `json:"has_logo"`
	LogoUpdatedAt int64 `json:"logo_updated_at"`
	Version       int64 `json:"version"`
}

// SiteAppearanceInput is what an owner may change, with the version it read.
type SiteAppearanceInput struct {
	Theme          string `json:"theme"`
	SEOTitle       string `json:"seo_title"`
	SEODescription string `json:"seo_description"`
	SEOKeywords    string `json:"seo_keywords"`
	AllowIndexing  bool   `json:"allow_indexing"`
	Version        int64  `json:"version"`
}

// loadAppearance reads the settings without an ownership check; callers have
// either verified the owner or are rendering the public storefront.
func loadAppearance(ctx context.Context, db *gorm.DB, siteID string) (SiteAppearance, error) {
	view := SiteAppearance{Theme: brandThemes[0], AllowIndexing: true}
	var record migrations.OEMSiteAppearanceRecord
	err := db.WithContext(ctx).Where("site_id = ?", siteID).First(&record).Error
	if err == nil {
		view = SiteAppearance{Theme: record.Theme, SEOTitle: record.SEOTitle, SEODescription: record.SEODescription, SEOKeywords: record.SEOKeywords, AllowIndexing: !record.NoIndex, Version: record.Version}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return view, err
	}
	var logo migrations.OEMSiteLogoRecord
	err = db.WithContext(ctx).Select("updated_at").Where("site_id = ?", siteID).First(&logo).Error
	if err == nil {
		view.HasLogo, view.LogoUpdatedAt = true, logo.UpdatedAt
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return view, err
	}
	return view, nil
}

func (s *SiteStore) Appearance(ctx context.Context, owner int, siteID string) (SiteAppearance, error) {
	if _, err := s.Get(ctx, owner, siteID); err != nil {
		return SiteAppearance{}, err
	}
	return loadAppearance(ctx, s.db, siteID)
}

// validSEOText allows single-line text up to limit characters.
func validSEOText(value string, limit int) bool {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > limit {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// UpdateAppearance saves theme and SEO together under a version check, so two
// editors cannot silently overwrite each other.
func (s *SiteStore) UpdateAppearance(ctx context.Context, owner int, siteID string, input SiteAppearanceInput) (SiteAppearance, error) {
	site, err := s.Get(ctx, owner, siteID)
	if err != nil {
		return SiteAppearance{}, err
	}
	if !slices.Contains(brandThemes, input.Theme) || input.Version < 0 || input.Version > (1<<53)-2 ||
		!validSEOText(input.SEOTitle, 120) || !validSEOText(input.SEODescription, 300) || !validSEOText(input.SEOKeywords, 200) {
		return SiteAppearance{}, ErrOEMInvalid
	}
	auditID, err := newOEMID()
	if err != nil {
		return SiteAppearance{}, err
	}
	now := time.Now().Unix()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if input.Version == 0 {
			// Reserve the row; every writer then runs the same version-zero CAS.
			seed := migrations.OEMSiteAppearanceRecord{SiteID: siteID, OwnerUserID: owner, Theme: brandThemes[0]}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
				return err
			}
		}
		changed := tx.Model(&migrations.OEMSiteAppearanceRecord{}).Where("site_id = ? AND version = ?", siteID, input.Version).Updates(map[string]any{
			"theme": input.Theme, "seo_title": input.SEOTitle, "seo_description": input.SEODescription, "seo_keywords": input.SEOKeywords,
			"no_index": !input.AllowIndexing, "version": input.Version + 1, "updated_at": now,
		})
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return ErrOEMConflict
		}
		return tx.Create(&migrations.OEMSiteAuditRecord{ID: auditID, SiteID: siteID, ActorUserID: owner, Action: "site.appearance_updated", Version: site.Version, CreatedAt: now}).Error
	})
	if err != nil {
		return SiteAppearance{}, err
	}
	return loadAppearance(ctx, s.db, siteID)
}

// checkLogo fully decodes the upload and returns its real content type. The
// declared type and file name are never trusted.
func checkLogo(data []byte) (string, bool) {
	if len(data) == 0 || len(data) > logoMaxBytes {
		return "", false
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > logoMaxPixels || config.Height > logoMaxPixels {
		return "", false
	}
	// The size is bounded above, so a full decode cannot be a decompression bomb.
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return "", false
	}
	switch format {
	case "png", "jpeg", "webp":
		return "image/" + format, true
	}
	return "", false
}

// SetLogo replaces the logo, or removes it when data is empty.
func (s *SiteStore) SetLogo(ctx context.Context, owner int, siteID string, data []byte) (SiteAppearance, error) {
	site, err := s.Get(ctx, owner, siteID)
	if err != nil {
		return SiteAppearance{}, err
	}
	contentType, action := "", "site.logo_removed"
	if len(data) != 0 {
		var ok bool
		if contentType, ok = checkLogo(data); !ok {
			return SiteAppearance{}, ErrOEMInvalid
		}
		action = "site.logo_updated"
	}
	auditID, err := newOEMID()
	if err != nil {
		return SiteAppearance{}, err
	}
	now := time.Now().Unix()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		removed := tx.Where("site_id = ?", siteID).Delete(&migrations.OEMSiteLogoRecord{})
		if removed.Error != nil {
			return removed.Error
		}
		if len(data) == 0 {
			if removed.RowsAffected == 0 {
				return nil // Nothing to remove.
			}
		} else if err := tx.Create(&migrations.OEMSiteLogoRecord{SiteID: siteID, OwnerUserID: owner, ContentType: contentType, Data: data, UpdatedAt: now}).Error; err != nil {
			return err
		}
		return tx.Create(&migrations.OEMSiteAuditRecord{ID: auditID, SiteID: siteID, ActorUserID: owner, Action: action, Version: site.Version, CreatedAt: now}).Error
	})
	if err != nil {
		return SiteAppearance{}, err
	}
	return loadAppearance(ctx, s.db, siteID)
}

// siteLogo returns the stored logo, or ErrOEMNotFound.
func siteLogo(ctx context.Context, db *gorm.DB, siteID string) (migrations.OEMSiteLogoRecord, error) {
	var logo migrations.OEMSiteLogoRecord
	err := db.WithContext(ctx).Where("site_id = ?", siteID).First(&logo).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return logo, ErrOEMNotFound
	}
	return logo, err
}

func (h siteHTTP) appearance(c *gin.Context) {
	value, err := h.store().Appearance(c.Request.Context(), c.GetInt("id"), c.Param("id"))
	oemResponse(c, value, err)
}

func (h siteHTTP) updateAppearance(c *gin.Context) {
	var input SiteAppearanceInput
	if c.ContentType() != "application/json" || common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10), &input) != nil {
		oemResponse(c, nil, ErrOEMInvalid)
		return
	}
	value, err := h.store().UpdateAppearance(c.Request.Context(), c.GetInt("id"), c.Param("id"), input)
	oemResponse(c, value, err)
}

// logo returns the owner's current logo as base64, for the console preview.
func (h siteHTTP) logo(c *gin.Context) {
	if _, err := h.store().Get(c.Request.Context(), c.GetInt("id"), c.Param("id")); err != nil {
		oemResponse(c, nil, err)
		return
	}
	logo, err := siteLogo(c.Request.Context(), h.store().db, c.Param("id"))
	if err != nil {
		oemResponse(c, nil, err)
		return
	}
	oemResponse(c, gin.H{"content_type": logo.ContentType, "image": base64.StdEncoding.EncodeToString(logo.Data)}, nil)
}

// setLogo takes {"image": "<base64>"}; an empty string removes the logo.
func (h siteHTTP) setLogo(c *gin.Context) {
	var input struct {
		Image string `json:"image"`
	}
	// Base64 of the 1 MiB limit plus the JSON envelope.
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, logoMaxBytes*4/3+1024))
	if err != nil || c.ContentType() != "application/json" || common.Unmarshal(body, &input) != nil {
		oemResponse(c, nil, ErrOEMInvalid)
		return
	}
	data, err := base64.StdEncoding.DecodeString(input.Image)
	if err != nil {
		oemResponse(c, nil, ErrOEMInvalid)
		return
	}
	value, err := h.store().SetLogo(c.Request.Context(), c.GetInt("id"), c.Param("id"), data)
	oemResponse(c, value, err)
}
