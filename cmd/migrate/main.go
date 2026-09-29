package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/dev-nathaniel/Adroit-platform/deployments/migrations"
	"github.com/dev-nathaniel/Adroit-platform/pkg/config"
	"github.com/dev-nathaniel/Adroit-platform/pkg/database"
)

// ANSI colors
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

func printUsage() {
	fmt.Printf(`%s%sAdroit Platform — Production Database Migration Runner%s

Usage:
  migrate <command> [arguments]

Commands:
  %sup%s             Apply all pending migrations
  %sdown%s [steps]   Rollback migrations (default: 1 step; use 'all' for full rollback)
  %sversion%s        Display current migration version and state
  %sforce%s <v>      Forcefully set migration version and clear dirty state
  %shelp%s           Show this help message

Configuration (via Environment Variables or Flags):
  DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME, DB_SSLMODE
  or --dsn "postgres://user:password@host:port/dbname?sslmode=disable"

Examples:
  migrate up
  migrate down 1
  migrate down all
  migrate version
  migrate force 1
`, colorBold, colorCyan, colorReset,
		colorGreen, colorReset,
		colorYellow, colorReset,
		colorCyan, colorReset,
		colorRed, colorReset,
		colorBold, colorReset,
	)
}

func main() {
	var customDSN string
	flag.StringVar(&customDSN, "dsn", "", "Custom PostgreSQL DSN connection string")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	command := args[0]

	// Determine DSN
	dsn := customDSN
	if dsn == "" {
		dsn = config.GetEnv("DATABASE_URL", "")
		if dsn == "" {
			dsn = database.LoadConfig().DSN()
		}
	}

	switch command {
	case "up":
		fmt.Printf("%sApplying pending migrations...%s\n", colorCyan, colorReset)
		if err := database.MigrateUp(dsn, migrations.FS); err != nil {
			fmt.Fprintf(os.Stderr, "%sMigration failed: %v%s\n", colorRed, err, colorReset)
			os.Exit(1)
		}
		version, dirty, err := database.MigrateVersion(dsn, migrations.FS)
		if err != nil {
			fmt.Printf("%sMigrations applied successfully.%s\n", colorGreen, colorReset)
		} else {
			fmt.Printf("%sMigrations applied successfully. Current version: %d (dirty: %t)%s\n", colorGreen, version, dirty, colorReset)
		}

	case "down":
		steps := 1
		if len(args) > 1 {
			if args[1] == "all" {
				steps = 0
			} else {
				parsed, err := strconv.Atoi(args[1])
				if err != nil || parsed <= 0 {
					fmt.Fprintf(os.Stderr, "%sInvalid step count '%s'. Must be a positive integer or 'all'.%s\n", colorRed, args[1], colorReset)
					os.Exit(1)
				}
				steps = parsed
			}
		}

		if steps == 0 {
			fmt.Printf("%sRolling back ALL migrations...%s\n", colorYellow, colorReset)
		} else {
			fmt.Printf("%sRolling back %d migration step(s)...%s\n", colorYellow, steps, colorReset)
		}

		if err := database.MigrateDown(dsn, migrations.FS, steps); err != nil {
			fmt.Fprintf(os.Stderr, "%sRollback failed: %v%s\n", colorRed, err, colorReset)
			os.Exit(1)
		}
		version, dirty, _ := database.MigrateVersion(dsn, migrations.FS)
		fmt.Printf("%sRollback complete. Current version: %d (dirty: %t)%s\n", colorGreen, version, dirty, colorReset)

	case "version", "status":
		version, dirty, err := database.MigrateVersion(dsn, migrations.FS)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%sFailed to get migration version: %v%s\n", colorRed, err, colorReset)
			os.Exit(1)
		}
		dirtyStr := "clean"
		if dirty {
			dirtyStr = fmt.Sprintf("%sDIRTY (recovery required)%s", colorRed, colorReset)
		}
		fmt.Printf("%sCurrent Migration Version:%s %d [%s]\n", colorBold, colorReset, version, dirtyStr)

	case "force":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "%sUsage: migrate force <version_number>%s\n", colorRed, colorReset)
			os.Exit(1)
		}
		targetVersion, err := strconv.Atoi(args[1])
		if err != nil || targetVersion < 0 {
			fmt.Fprintf(os.Stderr, "%sInvalid version number '%s'%s\n", colorRed, args[1], colorReset)
			os.Exit(1)
		}
		fmt.Printf("%sForcing version to %d and resetting dirty state...%s\n", colorYellow, targetVersion, colorReset)
		if err := database.MigrateForce(dsn, migrations.FS, targetVersion); err != nil {
			fmt.Fprintf(os.Stderr, "%sForce failed: %v%s\n", colorRed, err, colorReset)
			os.Exit(1)
		}
		fmt.Printf("%sVersion forced to %d successfully.%s\n", colorGreen, targetVersion, colorReset)

	case "help", "-h", "--help":
		printUsage()

	default:
		fmt.Fprintf(os.Stderr, "%sUnknown command: %s%s\n\n", colorRed, command, colorReset)
		printUsage()
		os.Exit(1)
	}
}
