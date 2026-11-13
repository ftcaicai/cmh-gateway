package migrations

import (
	"github.com/QuantumNous/new-api/ext/core"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ContinuationRecord is extension-owned encrypted storage, not a public task DTO.
// Only continuation owns its contents. No source, runtime JSON or credentials
// have a plaintext column. Size lets MySQL choose a text type above 64 KiB.
type ContinuationRecord struct {
	RefID      string `gorm:"column:ref_id;type:varchar(64);primaryKey"`
	BindingSHA string `gorm:"column:binding_sha;type:varchar(64);not null"`
	Digest     string `gorm:"column:digest;type:varchar(64);not null"`
	KeyID      string `gorm:"column:key_id;type:varchar(64);not null"`
	Ciphertext string `gorm:"column:ciphertext;size:6291456;not null"`
}

func (ContinuationRecord) TableName() string { return "cmh_task_continuations" }

// MigrateContinuation is explicit and unregistered. It migrates only this
// extension table; callers must separately verify fresh and repeated startup.
func MigrateContinuation(db *gorm.DB) error {
	if db == nil {
		return core.ErrContinuationUnavailable
	}
	if db.Session(&gorm.Session{Logger: logger.Discard}).AutoMigrate(&ContinuationRecord{}) != nil {
		return core.ErrContinuationUnavailable
	}
	return nil
}
