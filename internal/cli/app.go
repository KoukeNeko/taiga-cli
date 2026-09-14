package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/KoukeNeko/taiga-cli/internal/completioncache"
	"github.com/KoukeNeko/taiga-cli/internal/config"
	"github.com/KoukeNeko/taiga-cli/internal/credential"
	"github.com/KoukeNeko/taiga-cli/internal/output"
	"github.com/KoukeNeko/taiga-cli/internal/taiga"
	"github.com/spf13/cobra"
)

const environmentPrefix = "TAIGA_"

type App struct {
	In         io.Reader
	Out        io.Writer
	Err        io.Writer
	HTTPClient *http.Client
	Config     *config.Store
	GitLocal   *config.GitLocal
	// Credentials, when set, is the credential store to use whatever
	// --credential-store says; tests set it. Otherwise the store is built on
	// first use from the mode and CredentialDirectory.
	Credentials         credential.Store
	CredentialDirectory string
	CompletionCache     *completioncache.Store
	Getenv              func(string) string
	Cwd                 string
	// StdinTTY reports whether input is a terminal, which is what decides
	// whether a question may be asked at all. It is left unset outside tests,
	// where input is a buffer that could never answer one.
	StdinTTY func() bool

	global globalOptions
}

type globalOptions struct {
	Profile string
	APIURL  string
	Project string
	JSON    bool
	Fields  []string
	NoInput bool
	NoColor bool
	Quiet   bool
	Verbose bool
	// CredentialStore is --credential-store as given, empty when not.
	CredentialStore string
}

type Settings struct {
	Profile      string `json:"profile"`
	APIURL       string `json:"api_url"`
	Project      string `json:"project,omitempty"`
	Token        string `json:"-"`
	RefreshToken string `json:"-"`
	// CredentialSource is where Token came from: credentialFromEnvironment,
	// credentialFromKeyring, credentialFromFile, or empty when there is none.
	CredentialSource string `json:"-"`
	// CredentialFile is the credentials file when that is the source.
	CredentialFile string `json:"-"`
}

const (
	credentialFromEnvironment = "environment"
	credentialFromKeyring     = "keyring"
	credentialFromFile        = "file"
)

func New() (*App, error) {
	path, err := config.DefaultPath()
	if err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get current directory: %w", err)
	}
	// No overall deadline on the client: the request layer bounds each JSON
	// attempt itself and watches a transfer for stalling, so an attachment or
	// a dump is as long as it is rather than as long as thirty seconds allow.
	return &App{
		In:                  os.Stdin,
		Out:                 os.Stdout,
		Err:                 os.Stderr,
		HTTPClient:          &http.Client{},
		Config:              config.NewStore(path),
		GitLocal:            config.NewGitLocal(cwd),
		CredentialDirectory: filepath.Dir(path),
		CompletionCache:     completioncache.NewStore(completioncache.DefaultPath(path)),
		Getenv:              os.Getenv,
		Cwd:                 cwd,
	}, nil
}

func (a *App) Execute(ctx context.Context, args []string) int {
	root := a.rootCommand()
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return ExitSuccess
	}
	known, body := classifyError(err)
	renderer := a.renderer()
	_ = renderer.Failure(body)
	return known.ExitCode
}

// flushTable writes a table out and, when the page it holds is only part of
// the list, says so. A table that stops at the page size looks exactly like a
// list that ends there, and a reader who cannot tell the two apart draws
// conclusions from a fraction of the data. The notice goes to stderr so that
// the table itself stays pipeable, and every command that paginates has
// --limit, which is why that is the flag named. JSON output says none of this
// because it already carries the page.
func (a *App) flushTable(writer *tabwriter.Writer, shown, total int) error {
	if err := writer.Flush(); err != nil {
		return err
	}
	if a.global.Quiet || total <= shown {
		return nil
	}
	_, _ = fmt.Fprintf(a.Err, "showing %d of %d; use --limit to see more\n", shown, total)
	return nil
}

func (a *App) renderer() output.Renderer {
	return output.Renderer{Out: a.Out, Err: a.Err, JSON: a.global.JSON, Fields: a.global.Fields, Quiet: a.global.Quiet}
}

func (a *App) rootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "taiga",
		Short: "Manage Taiga projects from the command line",
		// An unattended caller reads --help, not the README, and schema is one
		// alphabetical entry among thirty-odd with nothing marking it as the
		// one that describes the rest.
		Long: "Manage Taiga projects from the command line.\n\n" +
			"--json emits a versioned contract on stdout and a structured error on stderr, under fixed exit codes.\n" +
			"`taiga schema <command>` prints that command's input and output JSON Schema with its safety and\n" +
			"idempotency, which is what an unattended caller needs to decide whether it may run.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			if len(a.global.Fields) > 0 && !a.global.JSON {
				return usageError("--fields requires --json")
			}
			if a.Getenv("NO_COLOR") != "" {
				a.global.NoColor = true
			}
			if _, err := a.credentialMode(); err != nil {
				return err
			}
			return nil
		},
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError(err.Error()) })
	root.SetOut(a.Out)
	root.SetErr(a.Err)
	flags := root.PersistentFlags()
	flags.StringVar(&a.global.Profile, "profile", "", "Taiga profile to use")
	flags.StringVar(&a.global.APIURL, "api-url", "", "complete Taiga API base URL")
	flags.StringVarP(&a.global.Project, "project", "p", "", "Taiga project slug")
	flags.BoolVar(&a.global.JSON, "json", false, "emit the versioned JSON contract")
	flags.StringSliceVar(&a.global.Fields, "fields", nil, "comma-separated JSON fields to include; pass an unknown name to list them")
	flags.BoolVar(&a.global.NoInput, "no-input", false, "never prompt for input")
	flags.BoolVar(&a.global.NoColor, "no-color", false, "disable color output")
	flags.BoolVarP(&a.global.Quiet, "quiet", "q", false, "suppress non-essential human output")
	flags.BoolVarP(&a.global.Verbose, "verbose", "v", false, "print redacted HTTP diagnostics to stderr")
	flags.StringVar(&a.global.CredentialStore, "credential-store", "", "where to keep credentials: auto (the OS keyring, or a file where there is none), keyring, file or none; also TAIGA_CREDENTIAL_STORE")
	_ = root.RegisterFlagCompletionFunc("profile", a.completeProfiles)
	_ = root.RegisterFlagCompletionFunc("project", a.completeProjects)
	root.AddCommand(
		a.versionCommand(),
		a.doctorCommand(),
		a.authCommand(),
		a.configCommand(),
		a.projectCommand(),
		a.memberCommand(),
		a.roleCommand(),
		a.webhookCommand(),
		a.customFieldCommand(),
		a.metadataCommand(),
		a.dueDateCommand(),
		a.swimlaneCommand(),
		a.tagCommand(),
		a.notificationCommand(),
		a.applicationCommand(),
		a.storageCommand(),
		a.integrationCommand(),
		a.epicCommand(),
		a.issueCommand(),
		a.storyCommand(),
		a.taskCommand(),
		a.sprintCommand(),
		a.wikiCommand(),
		a.wikiLinkCommand(),
		a.searchCommand(),
		a.timelineCommand(),
		a.statsCommand(),
		a.batchCommand(),
		a.commentCommand(),
		a.csvCommand(),
		a.attachmentCommand(),
		a.schemaCommand(),
		a.completionCommand(root),
	)
	return root
}

func (a *App) resolveSettings(ctx context.Context) (Settings, config.File, error) {
	cfg, err := a.Config.Load()
	if err != nil {
		return Settings{}, config.File{}, err
	}
	local := config.LocalValues{}
	if a.GitLocal != nil {
		values, localErr := a.GitLocal.Load(ctx)
		if localErr == nil {
			local = values
		} else if !errors.Is(localErr, config.ErrNotGitRepository) {
			return Settings{}, config.File{}, localErr
		}
	}
	profileName := firstNonEmpty(a.global.Profile, a.env("PROFILE"), local.Profile, cfg.CurrentProfile, config.DefaultProfileName())
	profileName, err = config.NormalizeProfileName(profileName)
	if err != nil {
		return Settings{}, config.File{}, validationError("invalid_profile", err.Error())
	}
	profile := cfg.Profiles[profileName]
	apiURL := firstNonEmpty(a.global.APIURL, a.env("API_URL"), profile.APIURL)
	if apiURL != "" {
		apiURL, err = taiga.NormalizeAPIURL(apiURL)
		if err != nil {
			return Settings{}, config.File{}, validationError("invalid_api_url", err.Error())
		}
	}
	project := firstNonEmpty(a.global.Project, a.env("PROJECT"), local.Project, profile.Project)
	settings := Settings{Profile: profileName, APIURL: apiURL, Project: project, Token: strings.TrimSpace(a.env("TOKEN"))}
	if settings.Token != "" {
		settings.CredentialSource = credentialFromEnvironment
		return settings, cfg, nil
	}
	if apiURL == "" {
		return settings, cfg, nil
	}
	store, err := a.credentials()
	if err != nil {
		return Settings{}, config.File{}, err
	}
	tokens, location, err := store.Get(credential.Account(profileName, apiURL))
	if errors.Is(err, credential.ErrNotFound) {
		return settings, cfg, nil
	}
	if err != nil {
		return Settings{}, config.File{}, err
	}
	settings.Token, settings.RefreshToken = tokens.AuthToken, tokens.RefreshToken
	settings.CredentialSource, settings.CredentialFile = credentialFromKeyring, location.File
	if location.File != "" {
		settings.CredentialSource = credentialFromFile
	}
	return settings, cfg, nil
}

// credentials returns the credential store --credential-store and
// TAIGA_CREDENTIAL_STORE select, building it the first time it is needed.
func (a *App) credentials() (credential.Store, error) {
	if a.Credentials != nil {
		return a.Credentials, nil
	}
	mode, err := a.credentialMode()
	if err != nil {
		return nil, err
	}
	// Without a directory the credentials file would land wherever the
	// command happened to run.
	if a.CredentialDirectory == "" {
		return nil, errors.New("no directory is configured for credentials")
	}
	a.Credentials = credential.NewStore(mode, a.CredentialDirectory)
	return a.Credentials, nil
}

func (a *App) credentialMode() (credential.Mode, error) {
	mode, err := credential.ParseMode(firstNonEmpty(a.global.CredentialStore, a.env("CREDENTIAL_STORE")))
	if err != nil {
		return "", usageError(err.Error())
	}
	return mode, nil
}

// refreshOptions lets a client refresh the stored credential for settings:
// under the store's lock, starting from whatever pair is stored by then, and
// saving the pair Taiga hands back. A credential from TAIGA_TOKEN carries no
// refresh token, so it gets none of this.
func (a *App) refreshOptions(settings Settings) ([]taiga.ClientOption, error) {
	if settings.RefreshToken == "" {
		return nil, nil
	}
	store, err := a.credentials()
	if err != nil {
		return nil, err
	}
	account := credential.Account(settings.Profile, settings.APIURL)
	save := func(authToken, refreshToken string) error {
		_, err := store.Set(account, credential.Tokens{AuthToken: authToken, RefreshToken: refreshToken})
		return err
	}
	lock := func(ctx context.Context) (string, string, func(), error) {
		unlock, err := store.Lock(ctx)
		if err != nil {
			return "", "", nil, err
		}
		tokens, _, err := store.Get(account)
		if err != nil {
			unlock()
			// Refreshing now would store a credential that a logout in
			// another process has just removed.
			if errors.Is(err, credential.ErrNotFound) {
				return "", "", nil, authRequired("the saved Taiga credential was removed while this command ran; run `taiga auth login`")
			}
			return "", "", nil, err
		}
		return tokens.AuthToken, tokens.RefreshToken, unlock, nil
	}
	return []taiga.ClientOption{taiga.WithRefreshToken(settings.RefreshToken, save), taiga.WithRefreshLock(lock)}, nil
}

func (a *App) client(ctx context.Context, requireToken bool) (*taiga.Client, Settings, error) {
	settings, _, err := a.resolveSettings(ctx)
	if err != nil {
		return nil, Settings{}, err
	}
	if settings.APIURL == "" {
		return nil, Settings{}, validationError("missing_api_url", "no Taiga API URL configured; run `taiga auth login --host <url>` or pass --api-url")
	}
	if requireToken && settings.Token == "" {
		return nil, Settings{}, authRequired("no Taiga credential available; run `taiga auth login` or set TAIGA_TOKEN")
	}
	options := []taiga.ClientOption{taiga.WithHTTPClient(a.HTTPClient), taiga.WithToken(settings.Token)}
	refresh, err := a.refreshOptions(settings)
	if err != nil {
		return nil, Settings{}, err
	}
	options = append(options, refresh...)
	if a.global.Verbose {
		options = append(options, taiga.WithVerbose(a.Err))
	}
	client, err := taiga.NewClient(settings.APIURL, options...)
	return client, settings, err
}

// env reads a setting from the tool's environment variable for name, so that
// TAIGA_TOKEN and its siblings override configured values.
func (a *App) env(name string) string {
	return strings.TrimSpace(a.Getenv(environmentPrefix + name))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
