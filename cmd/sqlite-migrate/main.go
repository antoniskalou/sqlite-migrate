package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	migrate "github.com/antoniskalou/sqlite-migrate"
	"github.com/urfave/cli/v3"
	_ "modernc.org/sqlite"
)

const defaultMigrationDir = "./sql"

type commandOptions struct {
	verbose bool
	noColor bool
	json    bool
	rootDir string
}

func newCommandOptions(cmd *cli.Command) commandOptions {
	return commandOptions{
		verbose: cmd.Bool("verbose"),
		noColor: cmd.Bool("no-color"),
		json:    cmd.Bool("json"),
		rootDir: cmd.String("root"),
	}
}

type commandContext struct {
	rootDir  string
	renderer renderer
	migrator *migrate.Migrator
	stdout   io.Writer
	stderr   io.Writer
}

func newCommandContext(
	cmd *cli.Command,
) commandContext {
	opts := newCommandOptions(cmd)

	stdout := cmd.Writer
	stderr := cmd.ErrWriter

	renderer := newRenderer(opts.noColor)
	logger := prettyLogger(renderer, stdout, stderr, opts.verbose)
	if opts.json {
		logger = jsonLogger(stdout)
	}

	migrator := migrate.New(os.DirFS(opts.rootDir), logger)
	return commandContext{
		rootDir:  opts.rootDir,
		renderer: renderer,
		migrator: migrator,
		stdout:   cmd.Writer,
		stderr:   cmd.ErrWriter,
	}
}

type commandActionFunc func(commandContext, *cli.Command) error

func withCommandContext(f commandActionFunc) cli.ActionFunc {
	return func(_ context.Context, cmd *cli.Command) error {
		return f(newCommandContext(cmd), cmd)
	}
}

func main() {
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
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "print SQL statements while migrating",
			},
			&cli.BoolFlag{
				Name:    "no-color",
				Usage:   "disable coloured output",
				Sources: cli.EnvVars("NO_COLOR"),
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "output all events as JSON",
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
				Usage: "initialise the SQL migration directory",
				Action: withCommandContext(
					func(ctx commandContext, cmd *cli.Command) error {
						return cmdInit(ctx)
					},
				),
			},
			{
				Name:      "status",
				Usage:     "print migration status of the database",
				Arguments: []cli.Argument{&dbPathArg},
				Action: withCommandContext(
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
					func(ctx commandContext, cmd *cli.Command) error {
						return cmdRollback(ctx, cmd.StringArg("dbpath"), cmd.Int("target"))
					},
				),
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		renderer := newRenderer(cmd.Bool("no-color"))
		fmt.Fprintf(cmd.ErrWriter, "%s %v\n", renderer(errorStyle, "[ERR]"), err)
		os.Exit(1)
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

	rootDir := ctx.rootDir
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

	fmt.Fprintf(ctx.stderr, "%s Migration directory initialized at %s\n",
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

	printStatus(ctx.renderer, ctx.stderr, status)

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
