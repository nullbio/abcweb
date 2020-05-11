package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	"github.com/spf13/afero"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/volatiletech/abcweb/v5/strmangle"
)

const (
	// AppConfigFilename is the filename for the app config file
	AppConfigFilename = "config.toml"
)

// AppFS is a handle to the filesystem in use
var AppFS = afero.NewOsFs()

// Configuration holds app state variables
type Configuration struct {
	// AppPath is the path to the project, set using the init function
	AppPath string

	// AppName is the name of the application, derived from the path
	AppName string

	// AppEnvName is the name of the app in environment variable prefix format
	// for example "MYAPP" instead of "MyApp".
	AppEnvName string

	// ActiveEnv is the environment mode currently set by "env" in config.toml
	// or APPNAME_ENV environment variable. This mode indicates what section of
	// config variables to to load into the config structs.
	ActiveEnv string

	// ModeViper is a *viper.Viper that has been initialized to:
	// Load the active environment section of the AppPath/config.toml file
	// Load environment variables with a prefix of APPNAME
	// Replace "-" with "_" in environment variable names
	ModeViper *viper.Viper
}

// Initialize the config
func Initialize(env *pflag.Flag) (*Configuration, error) {
	c := &Configuration{}

	path, err := getAppPath()
	if err != nil {
		return nil, err
	}
	c.AppPath = path

	c.AppName = getAppName(c.AppPath)
	c.AppEnvName = strmangle.EnvAppName(c.AppName)
	c.ActiveEnv = getActiveEnv(c.AppPath, c.AppName)
	// If ActiveEnv is not set via env var or config file,
	// OR the user has passed in an override value through a flag,
	// then set it to the flag value.
	if env != nil && (c.ActiveEnv == "" || env.Changed) {
		c.ActiveEnv = env.Value.String()
	}
	c.ModeViper = NewModeViper(c.AppPath, c.AppEnvName, c.ActiveEnv)

	return c, nil
}

// InitializeP the config but panic if anything goes wrong
func InitializeP(env *pflag.Flag) *Configuration {
	c, err := Initialize(env)
	if err != nil {
		panic(err)
	}

	return c
}

// DBConfig holds the configuration variables contained in the config.toml
// file for the environment currently loaded (obtained from GetDatabaseEnv())
type DBConfig struct {
	DB      string `toml:"db" mapstructure:"db"`
	Host    string `toml:"host" mapstructure:"host"`
	Port    int    `toml:"port" mapstructure:"port"`
	DBName  string `toml:"dbname" mapstructure:"dbname"`
	User    string `toml:"user" mapstructure:"user"`
	Pass    string `toml:"pass" mapstructure:"pass"`
	SSLMode string `toml:"sslmode" mapstructure:"sslmode"`
}

// AppConfig holds the relevant generated app config.toml file variables
type AppConfig struct {
	DefaultEnv string `toml:"env"`
}

var testHarnessViperReadConfig = func(newViper *viper.Viper) error {
	return newViper.ReadInConfig()
}

// NewModeViper creates a viper.Viper with config path and environment prefixes
// set. It also specifies a Sub of the active environment (the chosen env mode)
// and reads in the database config file section.
func NewModeViper(appPath string, envAppName, env string) *viper.Viper {
	newViper := viper.New()
	newViper.SetConfigType("toml")
	newViper.SetConfigFile(filepath.Join(appPath, AppConfigFilename))
	newViper.SetEnvPrefix(envAppName)
	newViper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	if env == "" {
		return newViper
	}

	// Do nothing on errors here, so we can fallback to other validation
	// methods. Users can use env vars or cmd line flags if a config is not found.
	err := testHarnessViperReadConfig(newViper)
	if err != nil {
		return newViper
	}

	modeViper := newViper.Sub(fmt.Sprintf("%s.db", env))
	if modeViper == nil {
		return newViper
	}

	return modeViper
}

// getActiveEnv attempts to get the config.toml environment
// to load by checking the following, in the following order:
// 1. environment variable $APPNAME_ENV (APPNAME is envAppName variable value)
// 2. config.toml default environment field "env"
func getActiveEnv(appPath, appName string) string {
	val := os.Getenv(strmangle.EnvAppName(appName) + "_ENV")
	if val != "" {
		return val
	}

	contents, err := afero.ReadFile(AppFS, filepath.Join(appPath, AppConfigFilename))
	if err != nil {
		return ""
	}

	var config AppConfig

	_, err = toml.Decode(string(contents), &config)
	if err != nil {
		return ""
	}

	return config.DefaultEnv
}

// getAppPath executes the git cmd "git rev-parse --show-toplevel" to obtain
// the full path of the current app. The last folder in the path is the app name.
func getAppPath() (string, error) {
	f, err := os.Stat(".abcweb.toml")
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("abcweb must be run from project root directory")
		}
		return "", err
	}

	if f.IsDir() {
		return "", errors.New(".abcweb.toml is a directory, not a file")
	}

	path, err := filepath.Abs(f.Name())
	if err != nil {
		return "", errors.Wrap(err, "cannot find absolute path to .abcweb.toml")
	}

	return filepath.Dir(path), nil
}

// getAppName gets the appname portion of a project path
func getAppName(appPath string) string {
	// Is "/" on both Windows and Linux
	split := strings.Split(appPath, "/")
	return split[len(split)-1]
}

// CheckEnv outputs an error if no ActiveEnv is found
func (c *Configuration) CheckEnv() error {
	if c.ActiveEnv == "" {
		return fmt.Errorf("No active environment chosen. Please choose an environment using the \"env\" flag in config.toml or the $%s_ENV environment variable", c.AppEnvName)
	}
	return nil
}
