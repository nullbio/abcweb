package cmd

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/volatiletech/abcweb/v5/cert"
	"github.com/volatiletech/abcweb/v5/strmangle"
)

var newCmdConfig newConfig

// newCmd represents the new command
var newCmd = &cobra.Command{
	Use:   "new [flags] <import_path> <abcweb_templates_path>",
	Short: "Generate a new abcweb app",
	Long: `The 'abcweb new' command generates a new abcweb application with a 
default directory structure and configuration at the Go src import path you specify.
`,
	Example: `abcweb new github.com/john/spinesnapper          -> generates app inside a ./spinesnapper folder
abcweb new github.com/john/spinesnapper/backend  -> generates app inside a ./backend folder
abcweb new gopkg.in/yaml                         -> generates app inside a ./yaml folder`,
	// Needs to be a persistentPreRunE to override root's config.Initialize call
	// otherwise abcweb needs to be run from the abcweb project or the git rev-parse
	// will cause a fatal error.
	PersistentPreRunE: newCmdPreRun,
	RunE:              newCmdRun,
}

func init() {
	newCmd.Flags().StringP("sessions-prod-storer", "p", "disk", "Session storer to use in production mode (cookie|memory|disk|redis)")
	newCmd.Flags().StringP("sessions-dev-storer", "d", "cookie", "Session storer to use in development mode (cookie|memory|disk|redis)")
	newCmd.Flags().StringP("tls-common-name", "", "localhost", "Common Name for generated TLS certificate")
	newCmd.Flags().StringP("default-env", "", "prod", "Default $APP_ENV to use when starting server")
	newCmd.Flags().StringP("bootstrap", "b", "regular", "Include Twitter Bootstrap 4 (none|regular|gridonly|rebootonly|gridandrebootonly)")
	newCmd.Flags().BoolP("no-gulp", "", false, "Skip generation of gulpfile.js, package.json and installation of gulp dependencies")
	newCmd.Flags().BoolP("no-bootstrap-js", "j", false, "Skip Twitter Bootstrap 4 javascript inclusion")
	newCmd.Flags().BoolP("no-font-awesome", "f", false, "Skip Font Awesome inclusion")
	newCmd.Flags().BoolP("no-livereload", "l", false, "Don't include LiveReload support")
	newCmd.Flags().BoolP("no-tls-certs", "t", false, "Skip generation of self-signed TLS cert files")
	newCmd.Flags().BoolP("no-readme", "r", false, "Skip README.md files")
	newCmd.Flags().BoolP("no-config", "c", false, "Skip default config.toml file")
	newCmd.Flags().BoolP("no-sessions", "s", false, "Skip support for http sessions")
	newCmd.Flags().BoolP("force-overwrite", "", false, "Force overwrite of existing files in your import_path")
	newCmd.Flags().BoolP("skip-npm-install", "", false, "Skip running npm install command")
	newCmd.Flags().BoolP("skip-git-init", "", false, "Skip running git init command")
	newCmd.Flags().BoolP("silent", "", false, "Disable console output")
	newCmd.Flags().BoolP("verbose", "v", false, "Show verbose output for npm install and dep ensure")

	RootCmd.AddCommand(newCmd)
}

func newCmdPreRun(cmd *cobra.Command, args []string) error {
	var err error

	viper.BindPFlags(cmd.Flags())

	newCmdConfig = newConfig{
		NoGulp:         viper.GetBool("no-gulp"),
		NoBootstrapJS:  viper.GetBool("no-bootstrap-js"),
		NoFontAwesome:  viper.GetBool("no-font-awesome"),
		NoLiveReload:   viper.GetBool("no-livereload"),
		NoSessions:     viper.GetBool("no-sessions"),
		NoTLSCerts:     viper.GetBool("no-tls-certs"),
		NoReadme:       viper.GetBool("no-readme"),
		NoConfig:       viper.GetBool("no-config"),
		ForceOverwrite: viper.GetBool("force-overwrite"),
		SkipNPMInstall: viper.GetBool("skip-npm-install"),
		SkipGitInit:    viper.GetBool("skip-git-init"),
		Silent:         viper.GetBool("silent"),
		ProdStorer:     viper.GetString("sessions-prod-storer"),
		DevStorer:      viper.GetString("sessions-dev-storer"),
		TLSCommonName:  viper.GetString("tls-common-name"),
		DefaultEnv:     viper.GetString("default-env"),
		Bootstrap:      strings.ToLower(viper.GetString("bootstrap")),
		Verbose:        viper.GetBool("verbose"),
	}

	validBootstrap := []string{"none", "regular", "gridonly", "rebootonly", "gridandrebootonly"}
	found := false
	for _, v := range validBootstrap {
		if newCmdConfig.Bootstrap == v {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("invalid bootstrap option (%q) supplied, valid options are: none|regular|gridonly|rebootonly|gridandrebootonly", newCmdConfig.Bootstrap)
	}

	newCmdConfig.AppPath, newCmdConfig.TemplatePath,
		newCmdConfig.ImportPath, newCmdConfig.AppName,
		newCmdConfig.AppEnvName, err = getAppPath(args)
	return err
}

func newCmdRun(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		cmd.Usage()
		os.Exit(1)
	}

	if !newCmdConfig.Silent {
		fmt.Println("Generating in:", newCmdConfig.AppPath)
	}

	// Make the app directory if it doesnt already exist.
	err := appFS.MkdirAll(newCmdConfig.AppPath, 0755)
	if err != nil {
		return err
	}

	// Make the empty folders that cannot be committed to git.
	for _, d := range emptyDirs {
		emptyDir := filepath.Join(newCmdConfig.AppPath, d)
		err := appFS.MkdirAll(emptyDir, 0755)
		if err != nil {
			return err
		}
	}

	// Walk all files in the templates folder
	err = afero.Walk(appFS, newCmdConfig.TemplatePath, func(path string, info os.FileInfo, err error) error {
		return newCmdWalk(newCmdConfig, newCmdConfig.TemplatePath, path, info, err)
	})
	if err != nil {
		return err
	}

	// Generate TLS certs if requested
	if !newCmdConfig.NoTLSCerts {
		err := generateTLSCerts(newCmdConfig)
		if err != nil {
			return err
		}
	}

	if err := goModInit(newCmdConfig); err != nil {
		return err
	}

	if !newCmdConfig.SkipGitInit {
		err = gitInit(newCmdConfig)
		if err != nil {
			return err
		}
	}

	if !newCmdConfig.NoGulp && !newCmdConfig.SkipNPMInstall {
		if !newCmdConfig.Silent {
			fmt.Printf("\n\tPlease note the `npm install` command can take a few minutes to complete.\n\tPlease be patient, generally the first run is the slowest.\n\n")
		}

		err = npmInstall(newCmdConfig)
		if err != nil {
			return err
		}
	}

	_, wireErr := exec.LookPath("wire")
	if wireErr == nil {
		fmt.Println("\trunning -> wire")
		cmd := exec.Command("wire")
		cmd.Dir = newCmdConfig.AppPath
		err = cmd.Run()
		if err != nil {
			return errors.Wrap(err, "wire failed")
		}
	}
	if !newCmdConfig.Silent {
		fmt.Printf("\tresult -> finished\n")
	}

	if wireErr != nil {
		fmt.Println("\n\tBefore your app will compile you will need to run wire. To install wire, please run 'abcweb deps' in your app folder.")
		fmt.Println("\tYou can run wire manually using 'wire' or 'go generate' once it is installed.")
	}

	return nil
}

func gitInit(cfg newConfig) error {
	if !cfg.Silent {
		fmt.Println("\trunning -> git init")
	}

	checkDep("git")

	exc := exec.Command("git", "init")
	exc.Dir = cfg.AppPath

	err := exc.Run()

	return err
}

func goModInit(cfg newConfig) error {
	if !cfg.Silent {
		fmt.Println("\trunning -> go mod init")
	}

	checkDep("go")

	var exc *exec.Cmd
	if cfg.Verbose {
		exc = exec.Command("go", "mod", "init", cfg.ImportPath)
		exc.Stdout = os.Stdout
	} else {
		exc = exec.Command("go", "mod", "init", cfg.ImportPath)
	}
	exc.Dir = cfg.AppPath

	err := exc.Run()

	return err
}

func npmInstall(cfg newConfig) error {
	if !cfg.Silent {
		fmt.Println("\trunning -> npm install")
	}

	checkDep("npm")

	var exc *exec.Cmd
	if cfg.Verbose {
		exc = exec.Command("npm", "install", "--verbose")
		exc.Stdout = os.Stdout
	} else {
		exc = exec.Command("npm", "install")
	}
	exc.Dir = cfg.AppPath

	err := exc.Run()

	return err
}

func generateTLSCerts(cfg newConfig) error {
	certFilePath := filepath.Join(cfg.AppPath, "cert.pem")
	privateKeyPath := filepath.Join(cfg.AppPath, "private.key")

	_, err := appFS.Stat(certFilePath)
	if err == nil || (err != nil && !os.IsNotExist(err)) {
		return nil
	}

	if !cfg.Silent {
		fmt.Println("\trunning -> TLS Certificate Generator")
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	template, err := cert.Template(cfg.AppName, cfg.TLSCommonName)
	if err != nil {
		return err
	}

	certFile, err := appFS.Create(certFilePath)
	if err != nil {
		return err
	}

	err = cert.WriteCertFile(certFile, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return err
	}
	if !cfg.Silent {
		fmt.Printf("\tcreate -> %s\n", filepath.Join(cfg.AppName, "cert.pem"))
	}

	privateKeyFile, err := appFS.Create(privateKeyPath)
	if err != nil {
		return err
	}

	if err := cert.WritePrivateKey(privateKeyFile, privateKey); err != nil {
		return err
	}
	if !cfg.Silent {
		fmt.Printf("\tcreate -> %s\n", filepath.Join(cfg.AppName, "private.key"))
	}

	return nil
}

func newCmdWalk(cfg newConfig, basePath string, path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	// Skip files and dirs depending on command line args
	if skip, err := processSkips(cfg, basePath, path, info); skip {
		return err
	}

	// Get the path for command line output, and the output target fullPath
	cleanPath, fullPath := getProcessedPaths(path, string(os.PathSeparator), cfg)

	fileContents := &bytes.Buffer{}

	// Check if the output file or folder already exists
	_, err = appFS.Stat(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	outputExists := err == nil

	// Dirs only get created if they don't already exist
	if info.IsDir() {
		// Don't bother trying to create the folder if it already exists.
		// Return now so we don't get "created" output
		if outputExists {
			return nil
		}

		err = appFS.MkdirAll(fullPath, 0755)
		if err != nil {
			return err
		}
	} else {
		// Files only get created if they don't already exist, or force overwrite is enabled
		if !cfg.ForceOverwrite && outputExists {
			return nil
		}

		rawFileContents, err := afero.ReadFile(appFS, path)
		if err != nil {
			return err
		}

		// Only process the file as a template if it has the .tmpl extension
		if strings.HasSuffix(path, ".tmpl") {
			t, err := template.New("").Funcs(templateFuncs).Parse(string(rawFileContents))
			if err != nil {
				return err
			}

			err = t.Execute(fileContents, cfg)
			if err != nil {
				return err
			}
		} else {
			if _, err := fileContents.Write(rawFileContents); err != nil {
				return err
			}
		}

		// Gofmt go files before save.
		// if strings.HasSuffix(fullPath, ".go") {
		// 	res, err := format.Source(fileContents.Bytes())
		// 	if err != nil {
		// 		return errors.Wrap(err, fmt.Sprintf("failed to format %s", fullPath))
		// 	}
		// 	fileContents.Reset()
		// 	if _, err := fileContents.Write(res); err != nil {
		// 		return err
		// 	}
		// }

		err = afero.WriteFile(appFS, fullPath, fileContents.Bytes(), 0644)
		if err != nil {
			return err
		}
	}

	if !cfg.Silent {
		fmt.Printf("\tcreate -> %s\n", cleanPath)
	}
	return nil
}

func processSkips(cfg newConfig, basePath string, path string, info os.FileInfo) (skip bool, err error) {
	// Ignore the root folder
	if path == basePath && info.IsDir() {
		return true, nil
	}

	// Skip directories defined in skipDirs slice
	if info.IsDir() {
		// Skip Twitter Bootstrap if requested
		if cfg.Bootstrap == "none" && info.Name() == "bootstrap" {
			return true, filepath.SkipDir
		} else if cfg.NoBootstrapJS && strings.HasSuffix(path, "/templates/assets/vendor/js/bootstrap") {
			return true, filepath.SkipDir
		}
		// Skip FontAwesome files if requested
		if cfg.NoFontAwesome && info.Name() == "font-awesome" {
			return true, filepath.SkipDir
		}

		for _, skipDir := range skipDirs {
			if info.Name() == skipDir {
				return true, filepath.SkipDir
			}
		}
	}

	// Skip sessions configuration if requested
	if cfg.NoSessions && strings.HasSuffix(path, "/templates/app/sessions.go.tmpl") {
		return true, nil
	}

	// Skip readme files if requested
	if cfg.NoReadme {
		if info.Name() == "README.md" || info.Name() == "README.md.tmpl" {
			return true, nil
		}
	}

	// Skip gulpjs if requested
	if cfg.NoGulp {
		if info.Name() == "gulpfile.js" || info.Name() == "gulpfile.js.tmpl" ||
			info.Name() == "package.json" || info.Name() == "package.json.tmpl" ||
			info.Name() == "manifest.json" {
			return true, nil
		}
	}

	// Skip livereload if requested
	if cfg.NoLiveReload {
		if info.Name() == "livereload.js" || info.Name() == "livereload.js.tmpl" {
			return true, nil
		}
	}

	// Skip default config.toml if requested
	if cfg.NoConfig {
		if info.Name() == "config.toml" || info.Name() == "config.toml.tmpl" {
			return true, nil
		}
	}

	var bsArr []string
	if cfg.Bootstrap == "none" {
		bsArr = bootstrapNone
	} else if cfg.Bootstrap == "regular" {
		bsArr = bootstrapRegular
	} else if cfg.Bootstrap == "gridonly" {
		bsArr = bootstrapGridOnly
	} else if cfg.Bootstrap == "rebootonly" {
		bsArr = bootstrapRebootOnly
	} else if cfg.Bootstrap == "gridandrebootonly" {
		bsArr = bootstrapGridRebootOnly
	}

	// Skip files contained within bsArr
	for _, bsFile := range bsArr {
		if info.Name() == bsFile || info.Name() == bsFile+".tmpl" {
			return true, nil
		}
	}

	// Skip Twitter Bootstrap JS files if requested
	if cfg.NoBootstrapJS {
		for _, bsFile := range bootstrapJSFiles {
			if info.Name() == bsFile || info.Name() == bsFile+".tmpl" {
				return true, nil
			}
		}
	}

	return false, nil
}

func getAppPath(args []string) (appPath, templatePath, importPath, appName, appEnvName string, err error) {
	if len(args) < 2 {
		return "", "", "", "", "", errors.New("must provide an import path and template path")
	}

	importPath = strings.Replace(args[0], `\`, "/", -1)
	// Somewhat validate provided import path, must have at least 2 components
	if !strings.ContainsRune(importPath, '/') {
		return "", "", "", "", "", errors.New("invalid import path provided, see --help for example")
	}

	appName = filepath.Base(importPath)
	appPath, err = filepath.Abs(appName)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("could not determine absolute path for project: %w", err)
	}
	templatePath = filepath.Clean(args[1])

	appEnvName = strmangle.EnvAppName(appName)

	return appPath, templatePath, importPath, appName, appEnvName, nil
}

func getProcessedPaths(path string, pathSeparator string, cfg newConfig) (cleanPath string, fullPath string) {
	chunks := strings.Split(path, pathSeparator)
	var newChunks []string

	var found int
	for i := 0; i < len(chunks); i++ {
		if chunks[i] == templatesDirectory {
			found = i
			break
		}
	}

	newChunks = append(newChunks, chunks[found:]...)

	// Make cleanPath for results output
	newChunks[0] = cfg.AppName
	newChunks[len(newChunks)-1] = strings.TrimSuffix(newChunks[len(newChunks)-1], ".tmpl")
	cleanPath = strings.Join(newChunks, string(os.PathSeparator))

	// Make fullPath for destination save
	newChunks[0] = cfg.AppPath
	fullPath = strings.Join(newChunks, string(os.PathSeparator))

	return cleanPath, fullPath
}
