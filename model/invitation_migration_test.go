package model

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupInvitationMigrationDB opens a real on-disk SQLite database, swaps it in
// as the package database, and restores the previous handle afterwards.
func setupInvitationMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "invitation-migration.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := DB, LOG_DB
	DB = db
	LOG_DB = db
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	return db
}

// preInvitationUserTables recreates the pre-feature tables that the shared
// migration expects to find, with the legacy users shape (no registration
// columns, no invitation_codes table).
func preInvitationUserTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE,
		password TEXT NOT NULL,
		display_name TEXT,
		role INTEGER DEFAULT 1,
		status INTEGER DEFAULT 1,
		email TEXT,
		quota INTEGER DEFAULT 0,
		used_quota INTEGER DEFAULT 0,
		aff_code TEXT UNIQUE,
		inviter_id INTEGER,
		created_at INTEGER,
		deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.AutoMigrate(
		&Channel{}, &Token{}, &UserSession{}, &AuthFlow{}, &ExternalIdentityClaim{},
		&PasskeyCredential{}, &Option{}, &LoginEncryptionKey{}, &Redemption{}, &Ability{},
		&Log{}, &Midjourney{}, &TopUp{}, &QuotaData{}, &Task{}, &TaskPlugin{},
		&Model{}, &Vendor{}, &PrefillGroup{}, &Setup{}, &TwoFA{}, &TwoFABackupCode{},
		&Checkin{}, &SubscriptionOrder{}, &UserSubscription{}, &SubscriptionPreConsumeRecord{},
		&CustomOAuthProvider{}, &UserOAuthBinding{}, &PerfMetric{}, &SystemInstance{},
		&SystemTask{}, &SystemTaskLock{}, &CasbinRule{}, &AuthzRole{}, &UserAccessToken{},
	))
	require.NoError(t, db.Exec(`INSERT INTO users (username, password, display_name, aff_code, inviter_id, created_at)
		VALUES ('migration-legacy-user', 'hash', 'Legacy User', 'MIG1', 0, 1)`).Error)
}

func assertInvitationSchemaPresent(t *testing.T, db *gorm.DB) {
	t.Helper()
	var count int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='invitation_codes'").Scan(&count).Error)
	assert.EqualValues(t, 1, count, "invitation_codes table must exist")
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND tbl_name='invitation_codes' AND name LIKE '%code_hash%'").Scan(&count).Error)
	assert.GreaterOrEqual(t, count, int64(1), "code_hash unique index must exist")
	for _, column := range []string{"register_method", "invitation_code_id"} {
		require.NoError(t, db.Raw("SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = ?", column).Scan(&count).Error)
		assert.EqualValues(t, 1, count, "users.%s must exist", column)
	}
}

// TestInvitationMigrationFreshDatabaseIsIdempotent verifies AutoMigrate on a
// real SQLite database: running the shared migration twice leaves the
// invitation schema in place and does not fail on the second pass.
func TestInvitationMigrationFreshDatabaseIsIdempotent(t *testing.T) {
	db := setupInvitationMigrationDB(t)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)

	require.NoError(t, migrateDB())
	assertInvitationSchemaPresent(t, db)

	require.NoError(t, migrateDB(), "second migration run must be idempotent")
	assertInvitationSchemaPresent(t, db)

	// Functional roundtrip on the migrated schema.
	codes, err := CreateInvitationCodes("migration-batch", 2, 0, 0)
	require.NoError(t, err)
	require.Len(t, codes, 2)
	user := &User{
		Username: "migration-new-user", Password: "password", AffCode: "MIGN",
		Status: common.UserStatusEnabled, RegisterMethod: common.RegisterMethodPassword,
	}
	require.NoError(t, db.Create(user).Error)
	consumed, err := ConsumeInvitationCodeWithTx(db, codes[0], user.Id)
	require.NoError(t, err)
	assert.Equal(t, user.Id, consumed.UsedUserId)
}

// TestInvitationMigrationUpgradePreservesExistingData verifies upgrading a
// representative pre-feature database: legacy rows survive, registration
// columns are added with empty defaults, and the new table appears.
func TestInvitationMigrationUpgradePreservesExistingData(t *testing.T) {
	db := setupInvitationMigrationDB(t)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	preInvitationUserTables(t, db)

	require.NoError(t, migrateDB())
	assertInvitationSchemaPresent(t, db)

	var userCount int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM users WHERE username = 'migration-legacy-user'").Scan(&userCount).Error)
	assert.EqualValues(t, 1, userCount, "legacy user row must be preserved")
	var method sql.NullString
	require.NoError(t, db.Raw("SELECT register_method FROM users WHERE username = 'migration-legacy-user'").Scan(&method).Error)
	assert.False(t, method.Valid, "legacy users have no recorded registration method")
}
