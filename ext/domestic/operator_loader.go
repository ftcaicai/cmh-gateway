package domestic

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// BuildOperatorManifestSHA256 may be injected by the extension release builder
// with -ldflags -X. It anchors exact file bytes, not the canonical policy
// fingerprint. An environment digest cannot override a nonempty build anchor.
var BuildOperatorManifestSHA256 string

// LoadOperatorPolicies reads only the two controlled entries of an explicit
// environment snapshot. It never installs policies or reads process environment.
// No configuration returns nil, nil; any partial or invalid configuration fails
// closed without retaining previous policies or exposing paths/content in errors.
//
// The file path is trusted operator configuration. Symlinks are followed and no
// filesystem authorization or approved-directory guarantee is made here. The
// returned Fingerprint identifies canonical policy semantics; the required SHA256
// anchor instead authenticates every raw file byte, including outer whitespace.
func LoadOperatorPolicies(environment map[string]string) (policies *OperatorPolicies, loadErr error) {
	path := environment["CMH_OPERATOR_MANIFEST_FILE"]
	environmentDigest := environment["CMH_OPERATOR_MANIFEST_SHA256"]
	buildDigest := BuildOperatorManifestSHA256
	if path == "" && environmentDigest == "" && buildDigest == "" {
		return nil, nil
	}
	if path == "" || environmentDigest == "" && buildDigest == "" {
		return nil, errGateConfiguration
	}
	for _, digest := range []string{environmentDigest, buildDigest} {
		if digest == "" {
			continue
		}
		if len(digest) != 64 {
			return nil, errGateConfiguration
		}
		for _, character := range digest {
			if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
				return nil, errGateConfiguration
			}
		}
	}
	if environmentDigest != "" && buildDigest != "" && environmentDigest != buildDigest {
		return nil, errGateConfiguration
	}
	expectedDigest := buildDigest
	if expectedDigest == "" {
		expectedDigest = environmentDigest
	}

	// Reject known non-regular paths before Open (a FIFO could otherwise block).
	// This trusted-operator path is not a race-proof filesystem sandbox.
	before, err := os.Stat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > maxOperatorManifestBytes {
		return nil, errGateConfiguration
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errGateConfiguration
	}
	defer func() {
		if file.Close() != nil {
			policies, loadErr = nil, errGateConfiguration
		}
	}()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxOperatorManifestBytes {
		return nil, errGateConfiguration
	}
	// Stat alone does not bound a file that grows while it is being read.
	document, err := io.ReadAll(io.LimitReader(file, maxOperatorManifestBytes+1))
	if err != nil || len(document) > maxOperatorManifestBytes {
		return nil, errGateConfiguration
	}
	digest := sha256.Sum256(document)
	if hex.EncodeToString(digest[:]) != expectedDigest {
		return nil, errGateConfiguration
	}
	policies, err = ParseOperatorManifest(document)
	if err != nil {
		return nil, errGateConfiguration
	}
	return policies, nil
}
