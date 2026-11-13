// Package continuation stores sensitive task artifacts without publishing or
// executing them. Host polling and financial recovery are not wired here.
package continuation

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/ext/core"
	"github.com/QuantumNous/new-api/ext/migrations"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const (
	maxSourceBytes     = 2 << 20
	maxRuntimeBytes    = 256 << 10
	maxPayloadBytes    = 4 << 20
	maxCiphertextBytes = 6 << 20
)

// Store retains a private copied keyring. It has no deletion, update, environment
// fallback or automatic key generation. Keep retired keys available for readers.
type Store struct {
	db          *gorm.DB
	activeKeyID string
	keys        map[string][]byte
}

var _ core.ContinuationStore = (*Store)(nil)

func New(db *gorm.DB, activeKeyID string, keys map[string][]byte) (*Store, error) {
	if db == nil || !validIdentifier(activeKeyID, 64) || len(keys) == 0 || len(keys) > 256 {
		return nil, core.ErrContinuationUnavailable
	}
	copied := make(map[string][]byte, len(keys))
	for id, key := range keys {
		if !validIdentifier(id, 64) || len(key) != 32 {
			return nil, core.ErrContinuationUnavailable
		}
		copied[id] = bytes.Clone(key)
	}
	if _, exists := copied[activeKeyID]; !exists {
		return nil, core.ErrContinuationUnavailable
	}
	// SQL diagnostics must not reveal ciphertext or sensitive arguments. The
	// public error contract also deliberately omits driver and keyring details.
	return &Store{db: db.Session(&gorm.Session{Logger: logger.Discard}), activeKeyID: activeKeyID, keys: copied}, nil
}

func validIdentifier(value string, limit int) bool {
	if len(value) == 0 || len(value) > limit {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func validBinding(binding core.ContinuationBinding) bool {
	if binding.TaskID == "" || len(binding.TaskID) > 128 || !utf8.ValidString(binding.TaskID) || binding.UserID <= 0 || binding.ChannelID <= 0 {
		return false
	}
	for _, c := range binding.TaskID {
		if unicode.IsControl(c) || unicode.IsSpace(c) {
			return false
		}
	}
	return true
}

func validDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// artifactPayload preserves exact source/runtime bytes. Runtime is only checked
// as a bounded UTF-8 JSON object: no schema, canonical-JSON or unique-key claim.
func artifactPayload(artifact core.ContinuationArtifact) ([]byte, error) {
	if !validIdentifier(artifact.PluginKey, 128) || !validDigest(artifact.SourceSHA256) || len(artifact.Source) == 0 || len(artifact.Source) > maxSourceBytes || !utf8.Valid(artifact.Source) || digest(artifact.Source) != artifact.SourceSHA256 || len(artifact.RuntimeJSON) == 0 || len(artifact.RuntimeJSON) > maxRuntimeBytes || !utf8.Valid(artifact.RuntimeJSON) {
		return nil, core.ErrContinuationUnavailable
	}
	var runtime map[string]common.RawMessage
	if common.Unmarshal(artifact.RuntimeJSON, &runtime) != nil || runtime == nil {
		return nil, core.ErrContinuationUnavailable
	}
	payload, err := common.Marshal(artifact)
	if err != nil || len(payload) > maxPayloadBytes {
		return nil, core.ErrContinuationUnavailable
	}
	return payload, nil
}

func reference(binding core.ContinuationBinding, payload []byte) (core.ContinuationRef, string, error) {
	bindingJSON, err := common.Marshal(binding)
	if err != nil {
		return core.ContinuationRef{}, "", core.ErrContinuationUnavailable
	}
	ref := core.ContinuationRef{Digest: digest(payload)}
	identity, err := common.Marshal(struct {
		Schema  int
		Binding core.ContinuationBinding
		Digest  string
	}{1, binding, ref.Digest})
	if err != nil {
		return core.ContinuationRef{}, "", core.ErrContinuationUnavailable
	}
	ref.ID = digest(identity)
	return ref, digest(bindingJSON), nil
}

func authenticatedData(binding core.ContinuationBinding, ref core.ContinuationRef, keyID string) ([]byte, error) {
	return common.Marshal(struct {
		Schema    int
		Binding   core.ContinuationBinding
		Reference core.ContinuationRef
		KeyID     string
	}{1, binding, ref, keyID})
}

func (store *Store) Save(ctx context.Context, binding core.ContinuationBinding, artifact core.ContinuationArtifact) (core.ContinuationRef, error) {
	if ctx == nil {
		return core.ContinuationRef{}, core.ErrContinuationUnavailable
	}
	if err := ctx.Err(); err != nil {
		return core.ContinuationRef{}, err
	}
	if store == nil || store.db == nil || !validBinding(binding) {
		return core.ContinuationRef{}, core.ErrContinuationUnavailable
	}
	payload, err := artifactPayload(artifact)
	if err != nil {
		return core.ContinuationRef{}, core.ErrContinuationUnavailable
	}
	ref, bindingSHA, err := reference(binding, payload)
	if err != nil {
		return core.ContinuationRef{}, core.ErrContinuationUnavailable
	}
	block, err := aes.NewCipher(store.keys[store.activeKeyID])
	if err != nil {
		return core.ContinuationRef{}, core.ErrContinuationUnavailable
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return core.ContinuationRef{}, core.ErrContinuationUnavailable
	}
	aad, err := authenticatedData(binding, ref, store.activeKeyID)
	if err != nil {
		return core.ContinuationRef{}, core.ErrContinuationUnavailable
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return core.ContinuationRef{}, core.ErrContinuationUnavailable
	}
	sealed := gcm.Seal(nonce, nonce, payload, aad)
	row := migrations.ContinuationRecord{RefID: ref.ID, BindingSHA: bindingSHA, Digest: ref.Digest, KeyID: store.activeKeyID, Ciphertext: base64.StdEncoding.EncodeToString(sealed)}
	if store.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error != nil {
		if err := ctx.Err(); err != nil {
			return core.ContinuationRef{}, err
		}
		return core.ContinuationRef{}, core.ErrContinuationUnavailable
	}
	// A primary-key collision must authenticate as the exact existing artifact;
	// never silently accept a conflict or rewrite an old encryption-key version.
	stored, err := store.Load(ctx, binding, ref, core.ContinuationPoll)
	if err != nil {
		return core.ContinuationRef{}, err
	}
	storedPayload, err := artifactPayload(stored)
	if err != nil || !bytes.Equal(payload, storedPayload) {
		return core.ContinuationRef{}, core.ErrContinuationUnavailable
	}
	return ref, nil
}

func (store *Store) Load(ctx context.Context, binding core.ContinuationBinding, ref core.ContinuationRef, purpose core.ContinuationPurpose) (core.ContinuationArtifact, error) {
	if ctx == nil {
		return core.ContinuationArtifact{}, core.ErrContinuationUnavailable
	}
	if err := ctx.Err(); err != nil {
		return core.ContinuationArtifact{}, err
	}
	if store == nil || store.db == nil || !validBinding(binding) || !validDigest(ref.ID) || !validDigest(ref.Digest) {
		return core.ContinuationArtifact{}, core.ErrContinuationUnavailable
	}
	switch purpose {
	case core.ContinuationPoll, core.ContinuationContent, core.ContinuationReport:
	default:
		return core.ContinuationArtifact{}, core.ErrContinuationUnavailable
	}
	bindingJSON, err := common.Marshal(binding)
	if err != nil {
		return core.ContinuationArtifact{}, core.ErrContinuationUnavailable
	}
	var row migrations.ContinuationRecord
	if store.db.WithContext(ctx).Where("ref_id = ? AND binding_sha = ? AND digest = ?", ref.ID, digest(bindingJSON), ref.Digest).Take(&row).Error != nil {
		if err := ctx.Err(); err != nil {
			return core.ContinuationArtifact{}, err
		}
		return core.ContinuationArtifact{}, core.ErrContinuationUnavailable
	}
	key, exists := store.keys[row.KeyID]
	if !exists || !validIdentifier(row.KeyID, 64) || len(row.Ciphertext) == 0 || len(row.Ciphertext) > maxCiphertextBytes {
		return core.ContinuationArtifact{}, core.ErrContinuationUnavailable
	}
	sealed, err := base64.StdEncoding.Strict().DecodeString(row.Ciphertext)
	if err != nil {
		return core.ContinuationArtifact{}, core.ErrContinuationUnavailable
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return core.ContinuationArtifact{}, core.ErrContinuationUnavailable
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(sealed) < gcm.NonceSize()+gcm.Overhead() || len(sealed) > maxPayloadBytes+gcm.NonceSize()+gcm.Overhead() {
		return core.ContinuationArtifact{}, core.ErrContinuationUnavailable
	}
	aad, err := authenticatedData(binding, ref, row.KeyID)
	if err != nil {
		return core.ContinuationArtifact{}, core.ErrContinuationUnavailable
	}
	payload, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], aad)
	if err != nil || len(payload) > maxPayloadBytes {
		return core.ContinuationArtifact{}, core.ErrContinuationUnavailable
	}
	var artifact core.ContinuationArtifact
	if common.Unmarshal(payload, &artifact) != nil {
		return core.ContinuationArtifact{}, core.ErrContinuationUnavailable
	}
	verifiedPayload, err := artifactPayload(artifact)
	if err != nil || !bytes.Equal(payload, verifiedPayload) {
		return core.ContinuationArtifact{}, core.ErrContinuationUnavailable
	}
	verifiedRef, _, err := reference(binding, verifiedPayload)
	if err != nil || verifiedRef != ref {
		return core.ContinuationArtifact{}, core.ErrContinuationUnavailable
	}
	artifact.Source, artifact.RuntimeJSON = bytes.Clone(artifact.Source), bytes.Clone(artifact.RuntimeJSON)
	return artifact, nil
}
