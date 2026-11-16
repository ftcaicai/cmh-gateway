// Command oem-migrate prepares only extension-owned OEM tables.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/QuantumNous/new-api/ext/migrations"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	engine := flag.String("engine", "", "sqlite, mysql or postgres")
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
	case "mysql":
		dialect = mysql.Open(dsn)
	case "postgres":
		dialect = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
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
	closeErr := sqlDB.Close()
	if err != nil || closeErr != nil {
		fmt.Fprintln(os.Stderr, "OEM migration failed")
		os.Exit(1)
	}
	fmt.Println("OEM extension tables ready")
}
