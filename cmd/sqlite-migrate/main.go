package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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

type commandContext struct {
	renderer renderer
	migrator *migrate.Migrator
}

func newCommandContext(
	cmd *cli.Command,
	opts loggerOptions,
) commandContext {
	renderer := newRenderer(opts.noColor)
	migrator := migrate.New(
		cmd.String("root"),
		logEvent(renderer, opts),
	)
	return commandContext{renderer, migrator}
}

type commandActionFunc func(commandContext, *cli.Command) error

func withCommandContext(opts loggerOptions, f commandActionFunc) cli.ActionFunc {
	return func(_ context.Context, cmd *cli.Command) error {
		return f(newCommandContext(cmd, opts), cmd)
	}
}

func main() {
	loggerOptions := loggerOptions{
		verbose: false,
		noColor: false,
	}

	dbPathArg := cli.StringArg{
		Name:     "dbpath",
		Required: true,
	}
	cmd := &cli.Command{
		Name:  "sqlite-migrate",
		Usage: "migrate SQLite databases",
		Copyright: `Copyright © 2026 Antonis Kalou

License: GNU Affero General Public License v3.0 or later (AGPL-3.0-or-later).
See https://www.gnu.org/licenses/agpl-3.0.html for details.`,
		Version: migrate.Version,
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
				Sources:     cli.EnvVars("NO_COLOR"),
			},
			&cli.StringFlag{
				Name:  "root",
				Usage: "root of the migration directory",
				Value: defaultMigrationDir,
			},
		},
		EnableShellCompletion: true,
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "initialize the SQL migration directory",
				Action: withCommandContext(
					loggerOptions,
					func(ctx commandContext, _ *cli.Command) error {
						return cmdInit(ctx)
					},
				),
			},
			{
				Name:      "status",
				Usage:     "print migration status of the database",
				Arguments: []cli.Argument{&dbPathArg},
				Action: withCommandContext(
					loggerOptions,
					func(ctx commandContext, cmd *cli.Command) error {
						return cmdStatus(ctx, cmd.StringArg("dbpath"))
					},
				),
			},
			{
				Name:      "deploy",
				Usage:     "apply all pending migrations",
				Arguments: []cli.Argument{&dbPathArg},
				Flags: []cli.Flag{&cli.IntFlag{
					Name:  "target",
					Usage: "specify target version",
				}},
				Action: withCommandContext(
					loggerOptions,
					func(ctx commandContext, cmd *cli.Command) error {
						var target *int
						if cmd.IsSet("target") {
							t := cmd.Int("target")
							target = &t
						}

						return cmdDeploy(ctx, cmd.StringArg("dbpath"), target)
					},
				),
			},
			{
				Name:      "rollback",
				Usage:     "rollback all applied migrations",
				Arguments: []cli.Argument{&dbPathArg},
				Flags: []cli.Flag{&cli.IntFlag{
					Name:     "target",
					Required: true,
					Usage:    "specify target version",
				}},
				Action: withCommandContext(
					loggerOptions,
					func(ctx commandContext, cmd *cli.Command) error {
						return cmdRollback(ctx, cmd.StringArg("dbpath"), cmd.Int("target"))
					},
				),
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		renderer := newRenderer(loggerOptions.noColor)
		fmt.Printf("%s %v\n", renderer(errorStyle, "[ERR]"), err)
		os.Exit(1)
	}
}

func logEvent(renderer renderer, options loggerOptions) func(migrate.Event) {
	return func(e migrate.Event) {
		switch e.Kind {
		case migrate.EventSkip:
			fmt.Printf("%s %s %s\n",
				renderer(skipStyle, "[SKIP]"),
				e.File,
				renderer(faintStyle, "(already applied)"),
			)
		case migrate.EventApply:
			dirStyle := upStyle
			if e.Direction == migrate.Down {
				dirStyle = downStyle
			}

			dir := strings.ToUpper(e.Direction.String())
			fmt.Printf("%s %s → user_version=%s\n",
				renderer(dirStyle, "["+dir+"]"),
				e.File,
				renderer(versionStyle, strconv.Itoa(e.Version)),
			)

			if options.verbose {
				fmt.Println(renderer(faintStyle, e.SQL))
			}
		case migrate.EventDone:
			fmt.Printf("%s user_version=%s\n",
				renderer(doneStyle, "[DONE]"),
				renderer(versionStyle, strconv.Itoa(e.Version)),
			)
		default:
			panic(fmt.Sprintf("unhandled event kind: %q", e.Kind))
		}
	}
}

func sqliteDSN(path string, options map[string]string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	if runtime.GOOS == "windows" {
		// convert windows path separators to URI separators for windows to work we
		// also need to append / before the root, e.g. C:\mydir becomes /C:/mydir
		absPath = "/" + filepath.ToSlash(absPath)
	}

	url := url.URL{
		Scheme: "file",
		Path:   absPath,
	}

	q := url.Query()
	for k, v := range options {
		q.Set(k, v)
	}
	url.RawQuery = q.Encode()

	return url.String(), nil
}

func connectDB(path string, preventCreate bool) (*sql.DB, error) {
	dsnOpts := make(map[string]string)
	if preventCreate {
		dsnOpts["mode"] = "rw"
	}

	dsn, err := sqliteDSN(path, dsnOpts)
	if err != nil {
		return nil, fmt.Errorf("invalid DSN format: %w", err)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	// fail early - Open only opens a handle, Ping checks that we can actually
	// connect. e.g. if the DB doesn't exist in rw mode, this will die here.
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func cmdInit(ctx commandContext) error {
	const sql = `-- code goes here`

	rootDir := ctx.migrator.RootDir()
	err := os.MkdirAll(rootDir, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create migration dir: %w", err)
	}

	up := filepath.Join(rootDir, "000_init.up.sql")
	if err := writeIfNotExists(up, sql); err != nil {
		return fmt.Errorf("failed to write %s migration: %w", up, err)
	}

	down := filepath.Join(rootDir, "000_init.down.sql")
	if err := writeIfNotExists(down, sql); err != nil {
		return fmt.Errorf("failed to write %s migration: %w", down, err)
	}

	absDir, err := filepath.Abs(rootDir)
	if err != nil {
		return fmt.Errorf("failed to resolve migration directory: %w", err)
	}

	fmt.Printf("%s Migration directory initialized at %s\n",
		ctx.renderer(doneStyle, "[DONE]"),
		absDir,
	)
	return nil
}

func cmdStatus(ctx commandContext, path string) error {
	db, err := connectDB(path, true)
	if err != nil {
		return fmt.Errorf("failed to open db at %s: %w", path, err)
	}
	defer db.Close()

	status, err := ctx.migrator.Status(db)
	if err != nil {
		return fmt.Errorf("failed to get db status: %w", err)
	}

	render := ctx.renderer

	applied := status.Applied
	pending := status.Pending

	lines := []string{
		render(titleStyle, "Database status"),
		"",
		fmt.Sprintf("%-10s %s", "Version:",
			render(versionStyle, strconv.Itoa(status.Version))),
		fmt.Sprintf("%-10s %s", "Pending:",
			render(pendingStyle, strconv.Itoa(len(pending)))),
		"",
		render(sectionStyle, "Applied migrations:"),
	}

	if len(applied) == 0 {
		lines = append(lines, render(faintStyle, "  (none)"))
	} else {
		for _, migration := range applied {
			lines = append(
				lines,
				render(upStyle, "  ✓ ")+filepath.Base(migration),
			)
		}
	}

	lines = append(lines, "", render(sectionStyle, "Pending migrations:"))

	if len(pending) == 0 {
		lines = append(lines, render(faintStyle, "  (none)"))
	} else {
		for _, migration := range pending {
			lines = append(
				lines,
				render(pendingStyle, "  • ")+filepath.Base(migration),
			)
		}
	}

	fmt.Print(strings.Join(lines, "\n"))

	return nil
}

func cmdDeploy(ctx commandContext, path string, target *int) error {
	db, err := connectDB(path, false)
	if err != nil {
		return fmt.Errorf("failed to open db at %s: %w", path, err)
	}
	defer db.Close()

	if target == nil {
		_, err = ctx.migrator.DeployAll(db)
	} else {
		_, err = ctx.migrator.Deploy(db, *target)
	}
	if err != nil {
		return fmt.Errorf("deploy failed: %w", err)
	}

	return nil
}

func cmdRollback(ctx commandContext, path string, target int) error {
	db, err := connectDB(path, true)
	if err != nil {
		return fmt.Errorf("failed to open db at %s: %w", path, err)
	}
	defer db.Close()

	_, err = ctx.migrator.Rollback(db, target)
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
