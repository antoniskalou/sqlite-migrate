package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	migrate "github.com/antoniskalou/sqlite-migrate"
	"github.com/urfave/cli/v3"
	_ "modernc.org/sqlite"
)

const defaultMigrationDir = "sql"

type loggerOptions struct {
	verbose bool
	noColor bool
}

func main() {
	loggerOptions := loggerOptions{
		verbose: false,
		noColor: false,
	}

	migrator := &migrate.Migrator{
		RootDir: defaultMigrationDir,
		OnEvent: logEvent(&loggerOptions),
	}

	dbPathArg := cli.StringArg{
		Name:     "dbpath",
		Required: true,
	}
	cmd := &cli.Command{
		Name:  "sqlite-migrate",
		Usage: "migrate SQLite databases",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "verbose",
				Aliases:     []string{"v"},
				Usage:       "print SQL statements while migrating",
				Destination: &loggerOptions.verbose,
			},
			&cli.BoolFlag{
				Name:        "no-color",
				Usage:       "disable coloured output",
				Destination: &loggerOptions.noColor,
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "initialize the SQL migration directory",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return cmdInit()
				},
			},
			{
				Name:      "status",
				Usage:     "print migration status of the database",
				Arguments: []cli.Argument{&dbPathArg},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return cmdStatus(migrator, cmd.StringArg("dbpath"))
				},
			},
			{
				Name:      "deploy",
				Usage:     "apply all pending migrations",
				Arguments: []cli.Argument{&dbPathArg},
				Flags: []cli.Flag{&cli.IntFlag{
					Name:  "target",
					Usage: "specify target version",
				}},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					var target *int
					if cmd.IsSet("target") {
						t := cmd.Int("target")
						target = &t
					}
					return cmdDeploy(migrator, cmd.StringArg("dbpath"), target)
				},
			},
			{
				Name:      "rollback",
				Usage:     "rollback all applied migrations",
				Arguments: []cli.Argument{&dbPathArg},
				Flags: []cli.Flag{&cli.IntFlag{
					Name:  "target",
					Usage: "specify target version",
				}},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					var target *int
					if cmd.IsSet("target") {
						t := cmd.Int("target")
						target = &t
					}
					return cmdRollback(migrator, cmd.StringArg("dbpath"), target)
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func logEvent(options *loggerOptions) func(migrate.Event) {
	return func(e migrate.Event) {
		switch e.Kind {
		case migrate.EventSkip:
			fmt.Printf("%s %s %s\n",
				skipStyle.Render("[SKIP]"),
				e.File,
				faintStyle.Render("(already applied)"),
			)
		case migrate.EventApply:
			dirStyle := upStyle
			if e.Direction == migrate.Down {
				dirStyle = downStyle
			}

			dir := strings.ToUpper(e.Direction.String())
			fmt.Printf("%s %s %s\n",
				dirStyle.Render("["+dir+"]"),
				e.File,
				cyanStyle.Render(fmt.Sprintf(
					"→ user_version=%d", e.Version,
				)),
			)
		case migrate.EventSQL:
			if options.verbose {
				fmt.Println(faintStyle.Render(e.SQL))
			}
		case migrate.EventDone:
			fmt.Printf("%s user_version=%s\n",
				doneStyle.Render("[DONE]"),
				versionStyle.Render(strconv.Itoa(e.Version)),
			)
		default:
			panic(fmt.Sprintf("unhandled event kind: %q", e.Kind))
		}
	}
}

func connectDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open: %w", err)
	}
	// TODO: make sure path dir always exists
	// TODO: don't create the DB if it doesn't exist, unless we're running deploy

	return db, nil
}

func cmdInit() error {
	const sql = `-- code goes here`

	err := os.MkdirAll(defaultMigrationDir, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create migration dir: %w", err)
	}

	up := filepath.Join(defaultMigrationDir, "000_init.up.sql")
	if err := writeIfNotExists(up, sql); err != nil {
		return fmt.Errorf("failed to write %s migration: %w", up, err)
	}

	down := filepath.Join(defaultMigrationDir, "000_init.down.sql")
	if err := writeIfNotExists(down, sql); err != nil {
		return fmt.Errorf("failed to write %s migration: %w", down, err)
	}

	absDir, err := filepath.Abs(defaultMigrationDir)
	if err != nil {
		return fmt.Errorf("failed to resolve migration directory: %w", err)
	}

	fmt.Println("initialized migration directory at " + absDir)
	return nil
}

func cmdStatus(migrator *migrate.Migrator, path string) error {
	db, err := connectDB(path)
	if err != nil {
		return fmt.Errorf("failed to open db at %s: %w", path, err)
	}
	defer db.Close()

	status, err := migrator.Status(db)
	if err != nil {
		return fmt.Errorf("failed to get db status: %w", err)
	}

	// TODO: figure out nicer formatting
	fmt.Println("Version:", status.Version)

	fmt.Println("Applied:")
	if len(status.Applied) > 0 {
		fmt.Println("\t" + strings.Join(status.Applied, "\n\t"))
	} else {
		fmt.Println("\tNONE")
	}

	fmt.Println("Pending:")
	if len(status.Pending) > 0 {
		fmt.Println("\t" + strings.Join(status.Pending, "\n\t"))
	} else {
		fmt.Println("\tNONE")
	}

	return nil
}

func cmdDeploy(migrator *migrate.Migrator, path string, target *int) error {
	db, err := connectDB(path)
	if err != nil {
		return fmt.Errorf("failed to open db at %s: %w", path, err)
	}
	defer db.Close()

	_, err = migrator.Deploy(db, target)
	if err != nil {
		return fmt.Errorf("deploy failed: %w", err)
	}

	return nil
}

func cmdRollback(migrator *migrate.Migrator, path string, target *int) error {
	db, err := connectDB(path)
	if err != nil {
		return fmt.Errorf("failed to open db at %s: %w", path, err)
	}
	defer db.Close()

	_, err = migrator.Rollback(db, target)
	if err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	return nil
}

func writeIfNotExists(filename, content string) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil // file already exists, ignore
		}
		return err
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		_ = os.Remove(filename)
		return err
	}

	return nil
}
