package continuation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/ext/core"
	"github.com/QuantumNous/new-api/ext/migrations"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestContinuationArchiveDatabases(t *testing.T) {
	for _, engine := range []struct{ name, dsn string }{{"sqlite", t.TempDir() + "/continuation.sqlite"}, {"mysql", os.Getenv("CMH_TEST_MYSQL_DSN")}, {"postgres", os.Getenv("CMH_TEST_POSTGRES_DSN")}} {
		t.Run(engine.name, func(t *testing.T) {
			if engine.dsn == "" {
				t.Skip("dedicated continuation test database not configured")
			}
			var dialect gorm.Dialector
			switch engine.name {
			case "sqlite":
				dialect = sqlite.Open(engine.dsn)
			case "mysql":
				dialect = mysql.Open(engine.dsn)
			case "postgres":
				dialect = postgres.New(postgres.Config{DSN: engine.dsn, PreferSimpleProtocol: true})
			}
			db, err := gorm.Open(dialect, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			// Only the dedicated synthetic extension table belongs to this fixture.
			require.False(t, db.Migrator().HasTable(&migrations.ContinuationRecord{}), "test requires a fresh dedicated archive table, never an existing deployment")
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&migrations.ContinuationRecord{})) })
			require.NoError(t, migrations.MigrateContinuation(db))
			require.NoError(t, migrations.MigrateContinuation(db))
			var version string
			if engine.name == "sqlite" {
				require.NoError(t, db.Raw("SELECT sqlite_version()").Scan(&version).Error)
			} else {
				require.NoError(t, db.Raw("SELECT version()").Scan(&version).Error)
			}
			t.Logf("real archive database: %s %s", engine.name, version)
			for _, bad := range []struct {
				active string
				keys   map[string][]byte
			}{{"", nil}, {"missing", map[string][]byte{"first": bytes.Repeat([]byte{1}, 32)}}, {"bad/key", map[string][]byte{"bad/key": bytes.Repeat([]byte{1}, 32)}}, {"first", map[string][]byte{"first": []byte("short")}}} {
				invalid, err := New(db, bad.active, bad.keys)
				require.ErrorIs(t, err, core.ErrContinuationUnavailable)
				assert.Nil(t, invalid)
			}
			// Fixed bytes are synthetic fixture keys only; no production key is
			// generated, persisted or configured by this unregistered archive.
			key := bytes.Repeat([]byte{0x42}, 32)
			keys := map[string][]byte{"first": key}
			store, err := New(db, "first", keys)
			require.NoError(t, err)
			binding := core.ContinuationBinding{TaskID: "synthetic-task-A", UserID: 11, ChannelID: 7}
			source := append([]byte("export const fixture = 'original artifact A';\n//"), bytes.Repeat([]byte("a"), 70*1024)...)
			artifact := core.ContinuationArtifact{PluginKey: "cmh-archive-fixture", SourceSHA256: fmt.Sprintf("%x", sha256.Sum256(source)), Source: source, RuntimeJSON: []byte(`{"version":1,"config":{"fixture":"private-runtime-A"},"endpoint":"https://fixture.invalid/path?synthetic=secret"}`)}
			ref, err := store.Save(t.Context(), binding, artifact)
			require.NoError(t, err)
			require.Len(t, ref.ID, 64)
			require.Len(t, ref.Digest, 64)
			again, err := store.Save(t.Context(), binding, artifact)
			require.NoError(t, err)
			assert.Equal(t, ref, again)
			require.NoError(t, migrations.MigrateContinuation(db))
			require.NoError(t, migrations.MigrateContinuation(db))
			reopened, err := gorm.Open(dialect, &gorm.Config{})
			require.NoError(t, err)
			reopenedSQL, err := reopened.DB()
			require.NoError(t, err)
			reopenedSQL.SetMaxOpenConns(1)
			defer func() { require.NoError(t, reopenedSQL.Close()) }()
			restarted, err := New(reopened, "first", keys)
			require.NoError(t, err)
			for _, purpose := range []core.ContinuationPurpose{core.ContinuationPoll, core.ContinuationContent, core.ContinuationReport} {
				loaded, err := restarted.Load(t.Context(), binding, ref, purpose)
				require.NoError(t, err)
				assert.Equal(t, artifact, loaded)
			}
			for _, wrong := range []core.ContinuationBinding{{TaskID: "synthetic-task-B", UserID: 11, ChannelID: 7}, {TaskID: binding.TaskID, UserID: 12, ChannelID: 7}, {TaskID: binding.TaskID, UserID: 11, ChannelID: 8}} {
				_, err := restarted.Load(t.Context(), wrong, ref, core.ContinuationPoll)
				require.ErrorIs(t, err, core.ErrContinuationUnavailable)
			}
			_, err = restarted.Load(t.Context(), binding, ref, core.ContinuationPurpose("submit"))
			require.ErrorIs(t, err, core.ErrContinuationUnavailable)
			wrongRef := ref
			wrongRef.Digest = fmt.Sprintf("%x", sha256.Sum256([]byte("other")))
			_, err = restarted.Load(t.Context(), binding, wrongRef, core.ContinuationPoll)
			require.ErrorIs(t, err, core.ErrContinuationUnavailable)
			otherKey := bytes.Repeat([]byte{0x73}, 32)
			wrongKeyStore, err := New(db, "first", map[string][]byte{"first": otherKey})
			require.NoError(t, err)
			_, err = wrongKeyStore.Load(t.Context(), binding, ref, core.ContinuationPoll)
			require.ErrorIs(t, err, core.ErrContinuationUnavailable)
			rotated, err := New(db, "second", map[string][]byte{"first": key, "second": otherKey})
			require.NoError(t, err)
			loaded, err := rotated.Load(t.Context(), binding, ref, core.ContinuationPoll)
			require.NoError(t, err)
			assert.Equal(t, artifact, loaded)
			withoutOldKey, err := New(db, "second", map[string][]byte{"second": otherKey})
			require.NoError(t, err)
			_, err = withoutOldKey.Load(t.Context(), binding, ref, core.ContinuationPoll)
			require.ErrorIs(t, err, core.ErrContinuationUnavailable)
			binding2 := binding
			binding2.TaskID = "synthetic-task-second-key"
			secondRef, err := rotated.Save(t.Context(), binding2, artifact)
			require.NoError(t, err)
			loaded, err = withoutOldKey.Load(t.Context(), binding2, secondRef, core.ContinuationPoll)
			require.NoError(t, err)
			assert.Equal(t, artifact, loaded)
			loaded.Source[0] ^= 0xff
			loaded.RuntimeJSON[0] ^= 0xff
			loaded, err = withoutOldKey.Load(t.Context(), binding2, secondRef, core.ContinuationPoll)
			require.NoError(t, err)
			assert.Equal(t, artifact, loaded)
			// Mutating constructor inputs must not replace the Store's encryption key.
			key[0] ^= 0xff
			loaded, err = store.Load(t.Context(), binding, ref, core.ContinuationPoll)
			require.NoError(t, err)
			assert.Equal(t, artifact, loaded)
			key[0] ^= 0xff
			badArtifact := artifact
			badArtifact.Source = []byte("replacement source without matching digest")
			_, err = store.Save(t.Context(), binding, badArtifact)
			require.ErrorIs(t, err, core.ErrContinuationUnavailable)
			badArtifact = artifact
			badArtifact.RuntimeJSON = []byte(`null`)
			_, err = store.Save(t.Context(), binding, badArtifact)
			require.ErrorIs(t, err, core.ErrContinuationUnavailable)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			_, err = store.Load(ctx, binding, ref, core.ContinuationPoll)
			require.Error(t, err)
			var rows []migrations.ContinuationRecord
			require.NoError(t, db.Find(&rows).Error)
			require.Len(t, rows, 2)
			stored := fmt.Sprintf("%+v", rows[0])
			assert.NotContains(t, stored, string(source))
			assert.NotContains(t, stored, "private-runtime-A")
			assert.NotContains(t, stored, "synthetic=secret")
			var originalRow, secondRow migrations.ContinuationRecord
			require.NoError(t, db.Where("ref_id = ?", ref.ID).Take(&originalRow).Error)
			require.NoError(t, db.Where("ref_id = ?", secondRef.ID).Take(&secondRow).Error)
			require.NoError(t, db.Model(&migrations.ContinuationRecord{}).Where("ref_id = ?", ref.ID).Update("ciphertext", secondRow.Ciphertext).Error)
			_, err = store.Load(t.Context(), binding, ref, core.ContinuationPoll)
			require.ErrorIs(t, err, core.ErrContinuationUnavailable)
			require.NoError(t, db.Model(&migrations.ContinuationRecord{}).Where("ref_id = ?", ref.ID).Update("ciphertext", originalRow.Ciphertext).Error)
			loaded, err = store.Load(t.Context(), binding, ref, core.ContinuationPoll)
			require.NoError(t, err)
			assert.Equal(t, artifact, loaded)
			// This synthetic corruption is deliberately confined to the new table.
			require.NoError(t, db.Model(&migrations.ContinuationRecord{}).Where("ref_id = ?", ref.ID).Update("ciphertext", "corrupt").Error)
			_, err = store.Load(t.Context(), binding, ref, core.ContinuationPoll)
			require.ErrorIs(t, err, core.ErrContinuationUnavailable)
			_, err = store.Save(t.Context(), binding, artifact)
			require.ErrorIs(t, err, core.ErrContinuationUnavailable, "idempotent save cannot silently overwrite or heal a tampered immutable row")
		})
	}
}
