package brand

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/ext/migrations"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"gorm.io/gorm"
)

// Self-service brand domains (T2.5). A site has one custom domain. The owner
// points it at the site's own CNAME target, <challenge>.<CMH_BRAND_CNAME_ROOT>;
// once the public CNAME matches exactly, the domain serves that brand on every
// node without a restart. Changing the domain invalidates the old one at once.
//
// The gateway does not terminate TLS. Certificates are issued by the reverse
// proxy in front of it, which asks GET /api/ext/brand/tls-allowed first.

// SyncTopicBrandDomains tells every node to reload the verified host map.
const SyncTopicBrandDomains = "cmh_brand_domains"

// Domain check outcomes shown to the owner; "" means no failed check yet.
const (
	domainErrorMissing  = "cname_missing"
	domainErrorMismatch = "cname_mismatch"
	domainErrorTaken    = "host_taken"
)

type domainSettings struct {
	root     string
	reserved []string
}

// domainConfig reads the environment on first use, after main has loaded .env.
var domainConfig = sync.OnceValue(func() (config domainSettings) {
	config.root = hostName(strings.TrimSpace(os.Getenv("CMH_BRAND_CNAME_ROOT")))
	// The platform's own domains can never be claimed by a brand.
	for domain := range strings.SplitSeq(os.Getenv("CMH_PLATFORM_DOMAINS"), ",") {
		if domain = hostName(strings.TrimSpace(domain)); domain != "" {
			config.reserved = append(config.reserved, domain)
		}
	}
	if config.root != "" {
		config.reserved = append(config.reserved, config.root)
	}
	return config
})

// SiteDomain is the owner's view of a site's domain.
type SiteDomain struct {
	// Available is false while the operator has not configured a CNAME root.
	Available   bool   `json:"available"`
	Host        string `json:"host"`
	CNAMETarget string `json:"cname_target"`
	Verified    bool   `json:"verified"`
	VerifiedAt  int64  `json:"verified_at"`
	CheckedAt   int64  `json:"checked_at"`
	LastError   string `json:"last_error"`
}

func domainView(record migrations.OEMSiteDomainRecord) SiteDomain {
	root := domainConfig().root
	view := SiteDomain{Available: root != "", Host: record.Host, Verified: record.VerifiedHost != nil, VerifiedAt: record.VerifiedAt, CheckedAt: record.CheckedAt, LastError: record.LastError}
	if root != "" && record.Challenge != "" {
		view.CNAMETarget = record.Challenge + "." + root
	}
	return view
}

// normalizeDomain accepts a plain public DNS name: lower-case ASCII labels, at
// least two of them, no port, no IP address, and outside the platform's own
// domains. Internationalized names are entered in their punycode form.
func normalizeDomain(input string) (string, bool) {
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(input)), ".")
	if host == "" || len(host) > 253 || !strings.Contains(host, ".") {
		return "", false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return "", false
	}
	for label := range strings.SplitSeq(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
				return "", false
			}
		}
	}
	if last := host[strings.LastIndex(host, ".")+1:]; last[0] >= '0' && last[0] <= '9' || last == "localhost" || last == "local" || last == "internal" || last == "test" || last == "invalid" {
		return "", false
	}
	for _, reserved := range domainConfig().reserved {
		if host == reserved || strings.HasSuffix(host, "."+reserved) {
			return "", false
		}
	}
	return host, true
}

// Domain returns the site's domain for its owner; a site without one yields an
// empty Host.
func (s *SiteStore) Domain(ctx context.Context, owner int, siteID string) (SiteDomain, error) {
	if _, err := s.Get(ctx, owner, siteID); err != nil {
		return SiteDomain{}, err
	}
	var record migrations.OEMSiteDomainRecord
	err := s.db.WithContext(ctx).Where("site_id = ?", siteID).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domainView(migrations.OEMSiteDomainRecord{}), nil
	}
	return domainView(record), err
}

// SetDomain saves, replaces or (with an empty host) removes the site's domain.
// Any change takes the previous domain out of service immediately and needs a
// fresh verification against a new challenge.
func (s *SiteStore) SetDomain(ctx context.Context, owner int, siteID, input string) (SiteDomain, error) {
	site, err := s.Get(ctx, owner, siteID)
	if err != nil {
		return SiteDomain{}, err
	}
	if domainConfig().root == "" {
		return SiteDomain{}, ErrOEMUnavailable
	}
	host, action := "", "site.domain_cleared"
	if strings.TrimSpace(input) != "" {
		var ok bool
		if host, ok = normalizeDomain(input); !ok {
			return SiteDomain{}, ErrOEMInvalid
		}
		action = "site.domain_set"
	}
	challenge, err := newOEMID()
	if err != nil {
		return SiteDomain{}, err
	}
	auditID, err := newOEMID()
	if err != nil {
		return SiteDomain{}, err
	}
	now := time.Now().Unix()
	record := migrations.OEMSiteDomainRecord{SiteID: siteID, OwnerUserID: owner, Host: host, Challenge: challenge, CreatedAt: now, UpdatedAt: now}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current migrations.OEMSiteDomainRecord
		found := tx.Where("site_id = ?", siteID).First(&current).Error
		if found != nil && !errors.Is(found, gorm.ErrRecordNotFound) {
			return found
		}
		if found == nil && current.Host == host {
			record = current // Saving the same domain again keeps its state.
			return nil
		}
		if err := tx.Where("site_id = ?", siteID).Delete(&migrations.OEMSiteDomainRecord{}).Error; err != nil {
			return err
		}
		if host == "" {
			record = migrations.OEMSiteDomainRecord{}
			if found != nil {
				return nil // Nothing to clear.
			}
		} else if err := tx.Create(&record).Error; err != nil {
			return err
		}
		return tx.Create(&migrations.OEMSiteAuditRecord{ID: auditID, SiteID: siteID, ActorUserID: owner, Action: action, Version: site.Version, CreatedAt: now}).Error
	})
	if err != nil {
		return SiteDomain{}, err
	}
	publishBrandHosts(ctx)
	return domainView(record), nil
}

// VerifyDomain checks the site's domain now, for its owner.
func (s *SiteStore) VerifyDomain(ctx context.Context, owner int, siteID string) (SiteDomain, error) {
	site, err := s.Get(ctx, owner, siteID)
	if err != nil {
		return SiteDomain{}, err
	}
	var record migrations.OEMSiteDomainRecord
	if err := s.db.WithContext(ctx).Where("site_id = ?", siteID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SiteDomain{}, ErrOEMNotFound
		}
		return SiteDomain{}, err
	}
	checked, err := s.checkDomain(ctx, record, site.Version, lookupCNAME)
	return domainView(checked), err
}

// lookupCNAME is the production resolver; tests replace it.
var lookupCNAME = net.DefaultResolver.LookupCNAME

// checkDomain resolves the domain's CNAME and, on an exact match with the
// site's target, marks it verified. The outcome is recorded either way. A
// domain another site already serves is refused. Re-checking a verified domain
// changes nothing.
func (s *SiteStore) checkDomain(ctx context.Context, record migrations.OEMSiteDomainRecord, siteVersion int64, lookup func(context.Context, string) (string, error)) (migrations.OEMSiteDomainRecord, error) {
	root := domainConfig().root
	if root == "" {
		return record, ErrOEMUnavailable
	}
	if record.VerifiedHost != nil {
		return record, nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	canonical, lookupErr := lookup(lookupCtx, record.Host+".")
	cancel()
	outcome := ""
	switch {
	case lookupErr != nil:
		outcome = domainErrorMissing
	case hostName(canonical) != record.Challenge+"."+root:
		outcome = domainErrorMismatch
	}
	auditID, err := newOEMID()
	if err != nil {
		return record, err
	}
	now := time.Now().Unix()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if outcome == "" {
			var taken int64
			if err := tx.Model(&migrations.OEMSiteDomainRecord{}).Where("verified_host = ? AND site_id <> ?", record.Host, record.SiteID).Count(&taken).Error; err != nil {
				return err
			}
			if taken != 0 {
				outcome = domainErrorTaken
			}
		}
		updates := map[string]any{"checked_at": now, "last_error": outcome, "updated_at": now}
		if outcome == "" {
			updates["verified_host"], updates["verified_at"] = record.Host, now
		}
		// The challenge in the WHERE clause drops a result for a domain that
		// was replaced while the lookup was running.
		changed := tx.Model(&migrations.OEMSiteDomainRecord{}).Where("site_id = ? AND challenge = ? AND verified_host IS NULL", record.SiteID, record.Challenge).Updates(updates)
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return ErrOEMConflict
		}
		if outcome != "" {
			return nil
		}
		return tx.Create(&migrations.OEMSiteAuditRecord{ID: auditID, SiteID: record.SiteID, ActorUserID: record.OwnerUserID, Action: "site.domain_verified", Version: siteVersion, CreatedAt: now}).Error
	})
	if err != nil {
		return record, err
	}
	record.CheckedAt, record.LastError = now, outcome
	if outcome == "" {
		record.VerifiedHost, record.VerifiedAt = &record.Host, now
		publishBrandHosts(ctx)
	}
	return record, nil
}

// verifyPendingDomains checks the unverified domains that were checked longest
// ago, a bounded batch per run, and returns how many became verified.
func (s *SiteStore) verifyPendingDomains(ctx context.Context) (int, error) {
	var pending []migrations.OEMSiteDomainRecord
	if err := s.db.WithContext(ctx).Where("verified_host IS NULL").Order("checked_at").Limit(20).Find(&pending).Error; err != nil {
		return 0, err
	}
	verified := 0
	for _, record := range pending {
		if ctx.Err() != nil {
			break
		}
		var site migrations.OEMSiteRecord
		if err := s.db.WithContext(ctx).Select("version").Where("id = ?", record.SiteID).First(&site).Error; err != nil {
			continue
		}
		checked, err := s.checkDomain(ctx, record, site.Version, lookupCNAME)
		if err == nil && checked.VerifiedHost != nil {
			verified++
		}
	}
	return verified, nil
}

// verifiedHosts is the host → site ID map every request consults. It is loaded
// on first use and replaced whenever the sync topic is bumped; the resolver
// never queries the database itself.
var (
	verifiedHosts     atomic.Pointer[map[string]string]
	verifiedHostsOnce sync.Once
)

// ReloadBrandHosts replaces the verified host map from the database.
func ReloadBrandHosts(ctx context.Context) error {
	if model.DB == nil {
		return ErrOEMUnavailable
	}
	var records []migrations.OEMSiteDomainRecord
	if err := model.DB.WithContext(ctx).Select("site_id", "verified_host").Where("verified_host IS NOT NULL").Find(&records).Error; err != nil {
		return err
	}
	hosts := make(map[string]string, len(records))
	for _, record := range records {
		hosts[*record.VerifiedHost] = record.SiteID
	}
	verifiedHosts.Store(&hosts)
	return nil
}

// publishBrandHosts applies a domain change on this node now and tells the
// other nodes to reload.
func publishBrandHosts(ctx context.Context) {
	if err := ReloadBrandHosts(ctx); err != nil {
		common.SysError(common.LogText("brand domain reload failed: %s", err.Error()))
	}
	model.BumpSyncTopic(SyncTopicBrandDomains)
}

func verifiedSiteForHost(host string) string {
	verifiedHostsOnce.Do(func() {
		if err := ReloadBrandHosts(context.Background()); err != nil {
			// Typically the extension tables are not migrated yet: no custom
			// domain is served until a domain change or sync reloads the map.
			common.SysLog(common.LogText("brand domains not loaded: %s", err.Error()))
		}
	})
	hosts := verifiedHosts.Load()
	if hosts == nil {
		return ""
	}
	return (*hosts)[host]
}

// domainVerifyTask checks pending domains every minute on whichever node
// claims the system task, so owners do not have to press "check now".
type domainVerifyTask struct{}

func (domainVerifyTask) Type() string            { return "cmh_brand_domain_verify" }
func (domainVerifyTask) Enabled() bool           { return domainConfig().root != "" }
func (domainVerifyTask) Interval() time.Duration { return time.Minute }
func (domainVerifyTask) NewPayload() any         { return nil }

func (domainVerifyTask) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	verified, err := NewSiteStore(model.DB).verifyPendingDomains(ctx)
	status, message := model.SystemTaskStatusSucceeded, ""
	if err != nil {
		status, message = model.SystemTaskStatusFailed, err.Error()
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, map[string]int{"verified": verified}, message); err != nil {
		common.SysLog(common.LogText("brand domain verification result not saved: %s", err.Error()))
	}
}

// RegisterDomains wires the sync topic and the verification task. It is
// called once from ext.Register, before the node sync and task runner start.
func RegisterDomains() {
	service.RegisterSyncTopic(SyncTopicBrandDomains, ReloadBrandHosts)
	service.RegisterSystemTaskHandler(domainVerifyTask{})
}
