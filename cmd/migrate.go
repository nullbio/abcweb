package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kat-co/vala"
	"github.com/spf13/cobra"
	"github.com/volatiletech/abcweb/v5/config"
)

// migrateCmd represents the "migrate" command
var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run migration tasks (up, down, redo, status, version)",
	Long: `Run migration tasks on the migrations in your migrations directory.

Migrations can be generated by using the "abcweb gen migration" command.
`,
	Example: "abcweb migrate up\nabcweb migrate down",
}

var errNoMigrations = errors.New("No migrations to run")

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Migrate the database to the most recent version",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := migrateExec(cmd, args, "up")
		if err != nil && err != errNoMigrations {
			return err
		}

		return nil
	},
}

var upOneCmd = &cobra.Command{
	Use:   "upone",
	Short: "Migrate the database by one version",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := migrateExec(cmd, args, "upone")
		if err != nil && err != errNoMigrations {
			return err
		}

		return nil
	},
}

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Roll back the version by one",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := migrateExec(cmd, args, "down")
		if err != nil && err != errNoMigrations {
			return err
		}

		return nil
	},
}

var downAllCmd = &cobra.Command{
	Use:   "downall",
	Short: "Roll back all migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := migrateExec(cmd, args, "downall")
		if err != nil && err != errNoMigrations {
			return err
		}

		return nil
	},
}

var redoCmd = &cobra.Command{
	Use:   "redo",
	Short: "Down then up the latest migration",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := migrateExec(cmd, args, "redo")
		if err != nil && err != errNoMigrations {
			return err
		}

		return nil
	},
}

var redoAllCmd = &cobra.Command{
	Use:   "redoall",
	Short: "Down then up all migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := migrateExec(cmd, args, "redoall")
		if err != nil && err != errNoMigrations {
			return err
		}

		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Dump the migration status for the current database",
	RunE: func(cmd *cobra.Command, args []string) error {
		return migrateExec(cmd, args, "status")
	},
}

var dbVersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the current version of the database",
	RunE: func(cmd *cobra.Command, args []string) error {
		return migrateExec(cmd, args, "version")
	},
}

func init() {
	// migrate flags
	migrateCmd.PersistentFlags().StringP("env", "e", "dev", "The config.toml file environment to load")

	RootCmd.AddCommand(migrateCmd)
	migrateCmd.AddCommand(upCmd)
	migrateCmd.AddCommand(upOneCmd)
	migrateCmd.AddCommand(downCmd)
	migrateCmd.AddCommand(downAllCmd)
	migrateCmd.AddCommand(redoCmd)
	migrateCmd.AddCommand(redoAllCmd)
	migrateCmd.AddCommand(statusCmd)
	migrateCmd.AddCommand(dbVersionCmd)

	migrateCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Usually the RootCmd persistent pre-run does this init for us,
		// but since we have to override the persistent pre-run here
		// to provide configuration to all children commands, we have to
		// do the init ourselves.
		var err error
		cnf, err = config.Initialize(cmd.Flags().Lookup("env"))
		if err != nil {
			return err
		}

		cnf.ModeViper.BindPFlags(migrateCmd.PersistentFlags())
		cnf.ModeViper.BindPFlags(cmd.Flags())

		return nil
	}
}

func migrateExec(cmd *cobra.Command, args []string, subCmd string) error {
	checkDep("mig")

	migrateCmdConfig.DBName = cnf.ModeViper.GetString("dbname")
	migrateCmdConfig.User = cnf.ModeViper.GetString("user")
	migrateCmdConfig.Pass = cnf.ModeViper.GetString("pass")
	migrateCmdConfig.Host = cnf.ModeViper.GetString("host")
	migrateCmdConfig.Port = cnf.ModeViper.GetInt("port")
	migrateCmdConfig.SSLMode = cnf.ModeViper.GetString("sslmode")

	var connStr string
	if migrateCmdConfig.SSLMode == "" {
		migrateCmdConfig.SSLMode = "require"
		cnf.ModeViper.Set("sslmode", migrateCmdConfig.SSLMode)
	}

	if migrateCmdConfig.Port == 0 {
		migrateCmdConfig.Port = 5432
		cnf.ModeViper.Set("port", migrateCmdConfig.Port)
	}
	connStr = postgresConnStr(migrateCmdConfig)

	err := vala.BeginValidation().Validate(
		vala.StringNotEmpty(migrateCmdConfig.User, "user"),
		vala.StringNotEmpty(migrateCmdConfig.Host, "host"),
		vala.Not(vala.Equals(migrateCmdConfig.Port, 0, "port")),
		vala.StringNotEmpty(migrateCmdConfig.DBName, "dbname"),
		vala.StringNotEmpty(migrateCmdConfig.SSLMode, "sslmode"),
	).Check()

	if err != nil {
		return err
	}

	excArgs := []string{
		subCmd,
		"postgres",
		connStr,
	}

	exc := exec.Command("mig", excArgs...)
	exc.Dir = filepath.Join(cnf.AppPath, migrationsDirectory)

	out, err := exc.CombinedOutput()

	fmt.Print(string(out))
	if strings.HasPrefix(string(out), "No migrations to run") {
		return errNoMigrations
	}

	return err
}

// postgressConnStr returns a postgres connection string compatible with the
// Go pq driver package, in the format:
// user=bob password=secret host=1.2.3.4 port=5432 dbname=mydb sslmode=verify-full
func postgresConnStr(cfg migrateConfig) string {
	connStrs := []string{
		fmt.Sprintf("user=%s", cfg.User),
	}

	if len(cfg.Pass) > 0 {
		connStrs = append(connStrs, fmt.Sprintf("password=%s", cfg.Pass))
	}

	connStrs = append(connStrs, []string{
		fmt.Sprintf("host=%s", cfg.Host),
		fmt.Sprintf("port=%d", cfg.Port),
		fmt.Sprintf("dbname=%s", cfg.DBName),
	}...)

	if len(cfg.SSLMode) > 0 {
		connStrs = append(connStrs, fmt.Sprintf("sslmode=%s", cfg.SSLMode))
	}

	return strings.Join(connStrs, " ")
}
