package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/filestore"
	"github.com/ra1phdd/zev/internal/core/httpclient"
	"github.com/ra1phdd/zev/internal/entities/history/repository"
	"github.com/ra1phdd/zev/internal/entities/query/repository/factory"
	queryservice "github.com/ra1phdd/zev/internal/entities/query/service"
	"github.com/ra1phdd/zev/pkg/tui/list"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	doctorStatusOK      = "ok"
	doctorStatusWarning = "warning"
	doctorStatusError   = "error"
	doctorStatusSkipped = "skipped"
	doctorActionNone    = "none"
	doctorCheckNetwork  = "network"
	doctorCheckProvider = "provider"
	doctorCheckHistory  = "history"
	doctorCheckZevDir   = "zev_directory"
)

type doctorOptions struct {
	JSON, Print, Confirm bool
}

type doctorCheck struct {
	Name              string `json:"name"`
	Status            string `json:"status"`
	Message           string `json:"message"`
	ActionableMessage string `json:"actionable_message"` //nolint:tagliatelle // Stable machine-readable doctor schema.
}

type doctorReport struct {
	SchemaVersion     int           `json:"schema_version"` //nolint:tagliatelle // Stable machine-readable doctor schema.
	Status            string        `json:"status"`
	Checks            []doctorCheck `json:"checks"`
	ActionableMessage string        `json:"actionable_message"` //nolint:tagliatelle // Stable machine-readable doctor schema.
}

type doctorDeps struct {
	loadConfig      func() (*config.Config, error)
	checkDirectory  func(string) error
	checkHistory    func(context.Context, string) error
	findExecutable  func(string) (string, error)
	networkCheck    func(context.Context, *config.Config) error
	askConfirmation func(context.Context) (bool, error)
}

func newDoctorCmd() *cobra.Command {
	var options doctorOptions
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose local configuration and provider readiness",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDoctor(cmd, options, defaultDoctorDeps())
		},
	}
	cmd.Flags().BoolVar(&options.JSON, "json", false, "Output a stable JSON report")
	cmd.Flags().BoolVar(&options.Print, "print", false, "Print a human-readable report without prompting")
	cmd.Flags().BoolVar(&options.Confirm, "confirm", false, "Send one real provider request without prompting")
	return cmd
}

func defaultDoctorDeps() doctorDeps {
	return doctorDeps{
		loadConfig: config.Load,
		checkDirectory: func(path string) error {
			if err := filestore.ValidateDirectory(path); err != nil {
				return fmt.Errorf("validate ~/.zev: %w", err)
			}
			info, err := os.Stat(path)
			if err != nil {
				return fmt.Errorf("stat ~/.zev: %w", err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
				return errors.New("directory permissions are wider than 0700")
			}
			return nil
		},
		checkHistory:   repository.CheckReadable,
		findExecutable: exec.LookPath,
		networkCheck:   doctorNetworkCheck,
		askConfirmation: func(ctx context.Context) (bool, error) {
			if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
				return false, nil
			}
			selected, err := list.SelectContext(ctx, list.SelectOptions{
				Title: "Send one real request to the configured provider?",
				Items: []list.Item{
					list.NewSimpleItem("No", "Keep this diagnosis local"),
					list.NewSimpleItem("Yes", "Send one provider request"),
				},
			})
			if err != nil {
				return false, fmt.Errorf("confirmation input: %w", err)
			}
			return strings.EqualFold(selected.Title(), "yes"), nil
		},
	}
}

func runDoctor(cmd *cobra.Command, options doctorOptions, deps doctorDeps) error {
	report, cfg := collectDoctorReport(cmd.Context(), deps)
	interactive := doctorShouldRunNetwork(options, report) && !options.Confirm
	if interactive {
		if err := writeDoctorReport(cmd, report, options); err != nil {
			return err
		}
	}
	if doctorShouldRunNetwork(options, report) {
		addDoctorCheck(&report, doctorNetworkResult(cmd.Context(), cfg, options, deps))
	} else {
		addDoctorCheck(&report, doctorSkippedNetworkCheck())
	}
	finalizeDoctorReport(&report)
	if interactive {
		if err := writeDoctorNetworkResult(cmd, report); err != nil {
			return err
		}
	} else if err := writeDoctorReport(cmd, report, options); err != nil {
		return err
	}
	if report.Status == doctorStatusError {
		return errors.New("doctor found failing checks")
	}
	return nil
}

func writeDoctorNetworkResult(cmd *cobra.Command, report doctorReport) error {
	check := report.Checks[len(report.Checks)-1]
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s: %s (action: %s)\n", check.Status, check.Name, check.Message, check.ActionableMessage); err != nil {
		return fmt.Errorf("write doctor network result: %w", err)
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), report.ActionableMessage); err != nil {
		return fmt.Errorf("write doctor summary: %w", err)
	}
	return nil
}

func doctorShouldRunNetwork(options doctorOptions, report doctorReport) bool {
	if report.Status == doctorStatusError {
		return false
	}
	return options.Confirm || !options.JSON && !options.Print
}

func checkDoctorCLIModes() error {
	for _, flags := range []parsedFlags{{Print: true, Option: 1}, {JSON: true, Option: 1}, {Print: true, Option: 3, OptionSet: true}, {JSON: true, Option: 3, OptionSet: true}} {
		if err := validateMachineFlags(flags); err != nil {
			return err
		}
	}
	return nil
}

func doctorNetworkResult(ctx context.Context, cfg *config.Config, options doctorOptions, deps doctorDeps) doctorCheck {
	if cfg == nil {
		return doctorSkippedNetworkCheck()
	}
	if !options.Confirm {
		confirmed, err := deps.askConfirmation(ctx)
		if err != nil {
			return doctorCheck{Name: doctorCheckNetwork, Status: doctorStatusError, Message: "confirmation was not completed", ActionableMessage: "use --confirm for non-interactive verification"}
		}
		if !confirmed {
			return doctorSkippedNetworkCheck()
		}
	}
	if err := deps.networkCheck(ctx, cfg); err != nil {
		return doctorCheck{Name: doctorCheckNetwork, Status: doctorStatusError, Message: "provider request failed", ActionableMessage: "check the provider endpoint, model and credentials; no response body was retained"}
	}
	return doctorCheck{Name: doctorCheckNetwork, Status: doctorStatusOK, Message: "real provider request succeeded; credentials were not printed", ActionableMessage: doctorActionNone}
}

func doctorSkippedNetworkCheck() doctorCheck {
	return doctorCheck{Name: doctorCheckNetwork, Status: doctorStatusSkipped, Message: "not checked; no network request was sent", ActionableMessage: "rerun with --confirm to test the provider"}
}

//nolint:gocognit // This report intentionally enumerates independent local diagnostics in one stable order.
func collectDoctorReport(ctx context.Context, deps doctorDeps) (doctorReport, *config.Config) {
	report := doctorReport{SchemaVersion: 1, Checks: []doctorCheck{}}
	cfg, configErr := deps.loadConfig()
	if configErr != nil {
		addDoctorCheck(&report, doctorCheck{Name: "configuration", Status: doctorStatusError, Message: "configuration could not be loaded", ActionableMessage: "fix the configuration file or environment and rerun doctor"})
	} else {
		addDoctorCheck(&report, doctorCheck{Name: "configuration", Status: doctorStatusOK, Message: "configuration loaded", ActionableMessage: doctorActionNone})
	}
	if cfg != nil {
		appendProviderChecks(&report, cfg)
		if cfg.CommandExecutionMode == "inject" || cfg.CommandExecutionMode == "execute" {
			addDoctorCheck(&report, doctorCheck{Name: "execution_mode", Status: doctorStatusOK, Message: "execution mode is supported", ActionableMessage: doctorActionNone})
		} else {
			addDoctorCheck(&report, doctorCheck{Name: "execution_mode", Status: doctorStatusError, Message: "execution mode is invalid", ActionableMessage: "set COMMAND_EXECUTION_MODE to inject or execute"})
		}
		appendExecutableCheck(&report, deps.findExecutable, "shell", doctorShell(), "set SHELL or COMSPEC to an installed shell")
		appendExecutableCheck(&report, deps.findExecutable, "editor", doctorEditor(cfg), "set SCRIPT_EDITOR or EDITOR to an installed editor")
	} else {
		addDoctorCheck(&report, doctorCheck{Name: doctorCheckProvider, Status: doctorStatusSkipped, Message: "provider checks skipped because configuration is unavailable", ActionableMessage: "fix configuration first"})
	}
	zevDir := config.GetZevDir()
	if err := deps.checkDirectory(zevDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			addDoctorCheck(&report, doctorCheck{Name: doctorCheckZevDir, Status: doctorStatusWarning, Message: "~/.zev does not exist yet", ActionableMessage: "run zev setup or create a private ~/.zev directory"})
		} else {
			addDoctorCheck(&report, doctorCheck{Name: doctorCheckZevDir, Status: doctorStatusError, Message: "~/.zev is not accessible or private", ActionableMessage: "fix the directory type, ownership and permissions"})
		}
	} else {
		addDoctorCheck(&report, doctorCheck{Name: doctorCheckZevDir, Status: doctorStatusOK, Message: "~/.zev is accessible and private", ActionableMessage: doctorActionNone})
	}
	if err := deps.checkHistory(ctx, filepath.Join(zevDir, "history.db")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			addDoctorCheck(&report, doctorCheck{Name: doctorCheckHistory, Status: doctorStatusWarning, Message: "history database is not initialized", ActionableMessage: "run a query once to initialize local history"})
		} else {
			addDoctorCheck(&report, doctorCheck{Name: doctorCheckHistory, Status: doctorStatusError, Message: "history database could not be read", ActionableMessage: "check history.db permissions/schema or recover the legacy history file"})
		}
	} else {
		addDoctorCheck(&report, doctorCheck{Name: doctorCheckHistory, Status: doctorStatusOK, Message: "history database is readable", ActionableMessage: doctorActionNone})
	}
	if err := checkDoctorCLIModes(); err != nil {
		addDoctorCheck(&report, doctorCheck{Name: "cli_modes", Status: doctorStatusError, Message: "machine CLI modes are unavailable", ActionableMessage: "check zev --print, zev --json and --option parsing"})
	} else {
		addDoctorCheck(&report, doctorCheck{Name: "cli_modes", Status: doctorStatusOK, Message: "interactive and machine CLI modes are available", ActionableMessage: doctorActionNone})
	}
	if term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) {
		addDoctorCheck(&report, doctorCheck{Name: "interactive_cli", Status: doctorStatusOK, Message: "interactive terminal mode is available", ActionableMessage: doctorActionNone})
	} else {
		addDoctorCheck(&report, doctorCheck{Name: "interactive_cli", Status: doctorStatusWarning, Message: "interactive terminal mode is unavailable on current standard streams", ActionableMessage: "run without --json or --print from a terminal to answer the provider check"})
	}
	finalizeDoctorReport(&report)
	return report, cfg
}

func appendProviderChecks(report *doctorReport, cfg *config.Config) {
	var model, baseURL, key string
	keyRequired := true
	switch cfg.LLMProvider {
	case "openai", "opencode":
		model, baseURL, key = cfg.OpenAIModel, cfg.OpenAIBaseURL, cfg.OpenAIAPIKey
	case "gemini":
		model, baseURL, key = cfg.GeminiModel, cfg.GeminiBaseURL, cfg.GeminiAPIKey
	case "anthropic":
		model, baseURL, key = cfg.AnthropicModel, cfg.AnthropicBaseURL, cfg.AnthropicAPIKey
	case "ollama":
		model, baseURL = cfg.OllamaModel, cfg.OllamaBaseURL
		keyRequired = false
	default:
		addDoctorCheck(report, doctorCheck{Name: doctorCheckProvider, Status: doctorStatusError, Message: "selected provider is unsupported", ActionableMessage: "choose openai, gemini, anthropic, ollama or opencode"})
		return
	}
	addDoctorCheck(report, doctorCheck{Name: doctorCheckProvider, Status: doctorStatusOK, Message: "selected provider is supported", ActionableMessage: doctorActionNone})
	if strings.TrimSpace(model) == "" {
		addDoctorCheck(report, doctorCheck{Name: "model", Status: doctorStatusError, Message: "required model is missing", ActionableMessage: "configure the selected provider model"})
	} else {
		addDoctorCheck(report, doctorCheck{Name: "model", Status: doctorStatusOK, Message: "model is configured", ActionableMessage: doctorActionNone})
	}
	if keyRequired && strings.TrimSpace(key) == "" {
		addDoctorCheck(report, doctorCheck{Name: "api_key", Status: doctorStatusError, Message: "API key is not configured", ActionableMessage: "configure the selected provider API key; doctor does not print keys"})
	} else {
		message := "API key is configured; functionality is not verified without a network check"
		if !keyRequired {
			message = "API key is not required for this provider"
		}
		addDoctorCheck(report, doctorCheck{Name: "api_key", Status: doctorStatusOK, Message: message, ActionableMessage: "use --confirm to test the provider"})
	}
	if err := validateDoctorURL(baseURL); err != nil {
		addDoctorCheck(report, doctorCheck{Name: "base_url", Status: doctorStatusError, Message: "provider base URL is invalid", ActionableMessage: "configure an HTTP(S) base URL with a host"})
	} else {
		addDoctorCheck(report, doctorCheck{Name: "base_url", Status: doctorStatusOK, Message: "provider base URL is syntactically valid", ActionableMessage: "network reachability is not checked without confirmation"})
	}
}

func appendExecutableCheck(report *doctorReport, find func(string) (string, error), name, value, action string) {
	if _, err := find(value); err != nil {
		addDoctorCheck(report, doctorCheck{Name: name, Status: doctorStatusError, Message: name + " is not available", ActionableMessage: action})
		return
	}
	addDoctorCheck(report, doctorCheck{Name: name, Status: doctorStatusOK, Message: name + " is available", ActionableMessage: doctorActionNone})
}

func doctorShell() string {
	if value := os.Getenv("SHELL"); value != "" {
		return value
	}
	if value := os.Getenv("COMSPEC"); value != "" {
		return value
	}
	return "sh"
}

func doctorEditor(cfg *config.Config) string {
	if cfg.ScriptEditor != "" {
		return cfg.ScriptEditor
	}
	if value := os.Getenv("EDITOR"); value != "" {
		return value
	}
	return "nano"
}

func validateDoctorURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New("invalid provider URL")
	}
	return nil
}

func doctorNetworkCheck(ctx context.Context, cfg *config.Config) error {
	client := httpclient.New(cfg.HTTPHeaders, nil)
	provider, err := factory.NewProvider(cfg, client)
	if err != nil {
		return fmt.Errorf("create provider: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err = queryservice.New(provider).GetOptions(ctx, "Doctor diagnostic: suggest one harmless command that produces no output.")
	if err != nil {
		return fmt.Errorf("provider diagnostic request: %w", err)
	}
	return nil
}

func addDoctorCheck(report *doctorReport, check doctorCheck) {
	report.Checks = append(report.Checks, check)
}

func finalizeDoctorReport(report *doctorReport) {
	status := doctorStatusOK
	for _, check := range report.Checks {
		switch check.Status {
		case doctorStatusError:
			status = doctorStatusError
		case doctorStatusWarning, doctorStatusSkipped:
			if status == doctorStatusOK {
				status = doctorStatusWarning
			}
		}
		if status == doctorStatusError {
			break
		}
	}
	report.Status = status
	switch status {
	case doctorStatusError:
		report.ActionableMessage = "fix the failing checks and rerun zev doctor"
	case doctorStatusWarning:
		report.ActionableMessage = "static checks passed; network/provider functionality is not verified without confirmation"
	default:
		report.ActionableMessage = "all checks passed"
	}
}

func writeDoctorReport(cmd *cobra.Command, report doctorReport, options doctorOptions) error {
	if options.JSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(report) //nolint:wrapcheck // Cobra owns the output writer boundary.
	}
	for _, check := range report.Checks {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s: %s (action: %s)\n", check.Status, check.Name, check.Message, check.ActionableMessage); err != nil {
			return fmt.Errorf("write doctor report: %w", err)
		}
	}
	_, err := fmt.Fprintln(cmd.OutOrStdout(), report.ActionableMessage)
	if err != nil {
		return fmt.Errorf("write doctor summary: %w", err)
	}
	return nil
}
