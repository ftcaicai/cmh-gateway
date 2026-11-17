// Command oem-migrate prepares only extension-owned OEM tables.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/ext/brand"
	"github.com/QuantumNous/new-api/ext/migrations"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	engine := flag.String("engine", "", "sqlite, mysql or postgres")
	backfill := flag.Bool("backfill-main-auth", false, "with the gateway stopped, bind every valid official session and login flow to the main site (required once before CMH_BRAND_AUTH=1)")
	revokeBrand := flag.Bool("revoke-brand-sessions", false, "with the gateway stopped, revoke every brand-bound session (required before any start without CMH_BRAND_AUTH)")
	flag.Parse()
	dsn := os.Getenv("CMH_OEM_MIGRATION_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "CMH_OEM_MIGRATION_DSN is required")
		os.Exit(1)
	}
	var dialect gorm.Dialector
	switch *engine {
	case "sqlite":
		dialect = sqlite.Open(dsn)
		common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	case "mysql":
		dialect = mysql.Open(dsn)
		common.SetMainDatabaseType(common.DatabaseTypeMySQL)
	case "postgres":
		dialect = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
		common.SetMainDatabaseType(common.DatabaseTypePostgreSQL)
	default:
		fmt.Fprintln(os.Stderr, "a supported explicit engine is required")
		os.Exit(1)
	}
	db, err := gorm.Open(dialect, &gorm.Config{Logger: logger.Discard})
	if err != nil {
		fmt.Fprintln(os.Stderr, "OEM migration connection failed")
		os.Exit(1)
	}
	sqlDB, err := db.DB()
	if err != nil {
		fmt.Fprintln(os.Stderr, "OEM migration connection failed")
		os.Exit(1)
	}
	err = migrations.MigrateOEM(db)
	if err == nil && *backfill {
		var added int64
		if added, err = brand.BackfillMainAuthBindings(db); err == nil {
			fmt.Printf("main-site auth bindings added: %d\n", added)
		}
	}
	if err == nil && *revokeBrand {
		// The official revoke reads model.DB; this process has no Redis, so a
		// cached copy of a revoked session lives on until the cache TTL ends.
		model.DB = db
		var revoked int64
		if revoked, err = brand.RevokeBrandSessions(db); err == nil {
			fmt.Printf("brand sessions revoked: %d\n", revoked)
		}
	}
	closeErr := sqlDB.Close()
	if err != nil || closeErr != nil {
		fmt.Fprintln(os.Stderr, "OEM migration failed")
		os.Exit(1)
	}
	fmt.Println("OEM extension tables ready")
}
