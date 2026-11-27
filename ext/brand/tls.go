package brand

import (
	"cmp"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/ext/migrations"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// HTTPS readiness of verified domains (display only). The reverse proxy issues
// certificates on demand, so the gateway cannot know from its own state
// whether https://<domain> works. It finds out the way a visitor would: a TLS
// handshake to the domain with full certificate and host name verification.
// The result never gates serving; it tells the owner whether to expect the
// browser padlock and when the certificate runs out.

// TLS check outcomes shown to the owner; "" means the last check succeeded.
const (
	tlsErrorUnreachable = "tls_unreachable"
	tlsErrorUntrusted   = "tls_untrusted"
	tlsErrorPrivate     = "tls_private_address"
)

const (
	tlsRecheckReady   = 24 * time.Hour
	tlsRecheckPending = 10 * time.Minute
	tlsProbeTimeout   = 5 * time.Second
)

// probeTLS is the production prober; tests replace it. It returns the leaf
// certificate's expiry, or an error carrying one of the tlsError codes.
var probeTLS = func(ctx context.Context, host string) (time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, tlsProbeTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addrs) == 0 {
		return time.Time{}, errors.New(tlsErrorUnreachable)
	}
	// A verified domain could later be pointed at an internal address; never
	// let the gateway probe its own network on an owner's behalf.
	for _, addr := range addrs {
		if !publicAddress(addr) {
			return time.Time{}, errors.New(tlsErrorPrivate)
		}
	}
	dialer := tls.Dialer{NetDialer: &net.Dialer{}, Config: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}}
	conn, err := dialer.DialContext(ctx, "tcp", netip.AddrPortFrom(addrs[0], 443).String())
	if err != nil {
		var unknownAuthority x509.UnknownAuthorityError
		var invalid x509.CertificateInvalidError
		var hostname x509.HostnameError
		if errors.As(err, &unknownAuthority) || errors.As(err, &invalid) || errors.As(err, &hostname) {
			return time.Time{}, errors.New(tlsErrorUntrusted)
		}
		return time.Time{}, errors.New(tlsErrorUnreachable)
	}
	defer conn.Close()
	state := conn.(*tls.Conn).ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return time.Time{}, errors.New(tlsErrorUntrusted)
	}
	return state.PeerCertificates[0].NotAfter, nil
}

func publicAddress(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.IsGlobalUnicast() && !addr.IsPrivate() && !addr.IsLoopback() && !addr.IsLinkLocalUnicast()
}

// checkTLS probes one verified host and stores the outcome. ReadyAt keeps the
// time of the first success in an unbroken run of successes.
func (s *SiteStore) checkTLS(ctx context.Context, siteID, host string) error {
	notAfter, probeErr := probeTLS(ctx, host)
	return s.saveTLS(ctx, siteID, host, notAfter, probeErr)
}

// saveTLS stores the outcome of one probe.
func (s *SiteStore) saveTLS(ctx context.Context, siteID, host string, notAfter time.Time, probeErr error) error {
	now := time.Now().Unix()
	record := migrations.OEMSiteTLSRecord{SiteID: siteID, Host: host, CheckedAt: now}
	if probeErr != nil {
		record.LastError = probeErr.Error()
		if record.LastError != tlsErrorUntrusted && record.LastError != tlsErrorPrivate {
			record.LastError = tlsErrorUnreachable
		}
	} else {
		var previous migrations.OEMSiteTLSRecord
		err := s.db.WithContext(ctx).Where("site_id = ? AND host = ?", siteID, host).Take(&previous).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		record.ReadyAt, record.NotAfter = cmp.Or(previous.ReadyAt, now), notAfter.Unix()
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&record).Error
}

// checkDueTLS probes the verified domains whose last HTTPS check is due: never
// checked, checked for another host, failing and older than ten minutes, or
// ready and older than a day. A bounded batch per run.
func (s *SiteStore) checkDueTLS(ctx context.Context) (int, error) {
	now := time.Now()
	var due []migrations.OEMSiteDomainRecord
	err := s.db.WithContext(ctx).Model(&migrations.OEMSiteDomainRecord{}).
		Select("cmh_oem_site_domains.site_id", "cmh_oem_site_domains.verified_host").
		Joins("LEFT JOIN cmh_oem_site_tls ON cmh_oem_site_tls.site_id = cmh_oem_site_domains.site_id AND cmh_oem_site_tls.host = cmh_oem_site_domains.verified_host").
		Where("cmh_oem_site_domains.verified_host IS NOT NULL").
		Where("cmh_oem_site_tls.site_id IS NULL OR (cmh_oem_site_tls.ready_at = 0 AND cmh_oem_site_tls.checked_at < ?) OR cmh_oem_site_tls.checked_at < ?",
			now.Add(-tlsRecheckPending).Unix(), now.Add(-tlsRecheckReady).Unix()).
		Order("cmh_oem_site_domains.verified_at").Limit(5).Find(&due).Error
	if err != nil {
		return 0, err
	}
	// Probe together so slow hosts share one timeout; store one at a time.
	notAfter, probeErrs := make([]time.Time, len(due)), make([]error, len(due))
	var probes sync.WaitGroup
	for i, record := range due {
		probes.Go(func() { notAfter[i], probeErrs[i] = probeTLS(ctx, *record.VerifiedHost) })
	}
	probes.Wait()
	for i, record := range due {
		if err := s.saveTLS(ctx, record.SiteID, *record.VerifiedHost, notAfter[i], probeErrs[i]); err != nil {
			return i, err
		}
	}
	return len(due), nil
}
