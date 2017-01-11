package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var migrateCmdConfig migrateConfig

// migrateCmd represents the "migrate" command
var migrateCmd = &cobra.Command{
	Use:     "migrate",
	Short:   "Run migration tasks",
	Long:    "describe tasks etc...",
	Example: "example here",
	PreRunE: migrateCmdPreRun,
	RunE:    migrateCmdRun,
}

func init() {
	// migrate flags
	migrateCmd.PersistentFlags().StringP("dir", "d", migrationsDirectory, "Directory with migration files")
	migrateCmd.PersistentFlags().StringP("db", "b", "", `Valid options: (postgres|mysql) (default: config.toml "db" field value)`)
	migrateCmd.PersistentFlags().StringP("env", "e", "dev", `config.toml environment to load (default: will only use "dev" default if cannot find in $PROJECTNAME_ENV)`)

	RootCmd.AddCommand(migrateCmd)

	// Add migrate subcommands
	// up
	// down
	// redo
	// status
	// dbversion

	viper.BindPFlags(migrateCmd.Flags())
}

func migrateCmdPreRun(cmd *cobra.Command, args []string) error {
	var err error

	migrateCmdConfig = migrateConfig{
		Dir: viper.GetString("dir"),
		DB:  viper.GetString("db"),
		Env: viper.GetString("env"),
	}

	// get other fields here:
	// migrateCmdConfig.Host =
	// migrateCmdConfig.Port =
	// migrateCmdConfig.DBName =
	// migrateCmdConfig.User =
	// migrateCmdConfig.Pass =
	// migrateCmdConfig.SSLMode =

	return err
}

func migrateCmdRun(cmd *cobra.Command, args []string) error {
	return nil
}
