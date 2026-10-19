package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/urfave/cli/v3"
	_ "modernc.org/sqlite"
)

const defaultMigrationDir = "sql"

func main() {
	cmd := &cli.Command{
		Name:  "sqlite-migrate",
		Usage: "migrate SQLite databases",
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "initialize the SQL migration directory",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return cmdInit()
				},
			},
			{
				Name:  "status",
				Usage: "print migration status of the database",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					fmt.Println("status")
					return nil
				},
			},
			{
				Name:  "deploy",
				Usage: "apply all pending migrations",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return nil
				},
			},
			{
				Name:  "rollback",
				Usage: "rollback all applied migrations",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					fmt.Println("rollback")
					return nil
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func connectDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)

	if err != nil {
		return nil, fmt.Errorf("failed to open: %v", err)
	}

	// TODO: make sure path dir always exists

	return db, nil
}

func cmdInit() error {
	const sql = `begin;

	-- code goes here

	commit;
	`

	err := os.MkdirAll(defaultMigrationDir, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create migration dir: %v", err)
	}

	up := path.Join(defaultMigrationDir, "000_init.up.sql")
	if err := writeIfNotExists(up, sql); err != nil {
		return fmt.Errorf("failed to write "+up+" migration", err)
	}

	down := path.Join(defaultMigrationDir, "000_init.down.sql")
	if err := writeIfNotExists(down, sql); err != nil {
		return fmt.Errorf("failed to write "+down+" migration", err)
	}

	absDir, err := filepath.Abs(defaultMigrationDir)
	if err != nil {
		return fmt.Errorf("failed to resolve migration directory: %v", err)
	}

	fmt.Println("initialized migration directory at " + absDir)
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

	_, err = f.WriteString(content)
	return err
}
