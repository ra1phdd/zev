// Package cli provides Zev command routing and terminal presentation.
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"slices"

	"github.com/ra1phdd/zev/internal/core/config"
	"github.com/ra1phdd/zev/internal/core/locale"
	"github.com/spf13/cobra"
)

var verbosityLevel int

// NewRootCommand creates the command tree for the supplied build version.
func NewRootCommand(buildVersion string) *cobra.Command {
	return NewRootCommandContext(context.Background(), buildVersion)
}

// NewRootCommandContext creates the command tree bound to ctx for the supplied build version.
//
//nolint:contextcheck // The composition root intentionally injects the caller context into Cobra.
func NewRootCommandContext(ctx context.Context, buildVersion string) *cobra.Command {
	if ctx == nil {
		ctx = context.Background()
	}

	if configured, err := config.LoadUILanguage(); err == nil {
		locale.SetLanguage(configured, "")
	}
	rootCmd := &cobra.Command{
		Use:                "zev [query]",
		Short:              locale.Text(locale.KeyQueryNaturalLanguageTerminalCommandSuggestions),
		Long:               locale.Text(locale.KeyQueryZevHelpsYouRememberTerminalCommandsDescribing),
		Args:               cobra.ArbitraryArgs,
		RunE:               runQuery,
		SilenceUsage:       true,
		SilenceErrors:      true,
		DisableFlagParsing: true, // Preserve flags intended for aliases.
	}
	rootCmd.SetContext(ctx)
	// Find preserves pre-subcommand flags for the query's manual parser; Traverse
	// would discard them when DisableFlagParsing is set on the root.
	rootCmd.TraverseChildren = false
	// Keep Cobra's help flag while moving its internal command out of the public tree.
	rootCmd.SetHelpCommand(&cobra.Command{Use: "__zev_help [command]", Hidden: true})
	registerRootQueryFlags(rootCmd)

	var rootVerbosity int
	rootCmd.PersistentFlags().CountVarP(&rootVerbosity, "verbose", "v", locale.Text(locale.KeyQueryIncreaseVerbosityLevelVInfoVvDebug))
	rootCmd.PersistentFlags().String("proxy", "", locale.Text(locale.KeyQueryProxyModelAndWebRequests))

	rootCmd.AddCommand(newQueryCmd())
	rootCmd.AddCommand(newContinueCmd())
	rootCmd.AddCommand(newHistoryCmd())
	rootCmd.AddCommand(newSetupCmd())
	rootCmd.AddCommand(newProviderCmd())
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newAliasCmd())
	rootCmd.AddCommand(newScriptCmd())
	rootCmd.AddCommand(newVersionCmd(buildVersion))
	rootCmd.AddCommand(newRunCmd())
	rootCmd.AddCommand(newUpdateCmd(buildVersion))
	rootCmd.InitDefaultCompletionCmd()
	localizeCommandHelp(rootCmd)
	return rootCmd
}

func commandProxy(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	if flag := cmd.Flag("proxy"); flag != nil {
		value := flag.Value.String()
		if flag.Changed && value == "" {
			return "http://"
		}
		return value
	}
	return ""
}

type commandProxyContextKey struct{}

func contextWithCommandProxy(ctx context.Context, proxyURL string) context.Context {
	return context.WithValue(ctx, commandProxyContextKey{}, proxyURL)
}

func proxyFromContext(ctx context.Context) string {
	proxyURL, ok := ctx.Value(commandProxyContextKey{}).(string)
	if !ok {
		return ""
	}
	return proxyURL
}

// Execute runs a configured command tree and returns its process exit code.
func Execute(ctx context.Context, root *cobra.Command) int {
	if configured, err := config.LoadUILanguage(); err == nil {
		defer locale.SetLanguage(configured, "")()
	}
	err := managedStorageError(root.ExecuteContext(ctx))
	if err == nil {
		return 0
	}
	if isInteractiveCancellation(err) {
		return 0
	}
	if machineErr, ok := errors.AsType[*machineExitError](err); ok {
		if _, writeErr := fmt.Fprintf(root.ErrOrStderr(), locale.Text(locale.KeyQueryErrorMessage), err); writeErr != nil {
			slog.Error("Failed to write command error", "error", writeErr)
		}
		return machineErr.code
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		if _, writeErr := fmt.Fprintf(root.ErrOrStderr(), locale.Text(locale.KeyQueryErrorMessage), err); writeErr != nil {
			slog.Error("Failed to write command error", "error", writeErr)
		}
		return exitErr.ExitCode()
	}
	if _, writeErr := fmt.Fprintf(root.ErrOrStderr(), locale.Text(locale.KeyQueryErrorMessage), err); writeErr != nil {
		slog.Error("Failed to write command error", "error", writeErr)
	}
	return 1
}

func isInteractiveCancellation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, errQueryCancelled) {
		return true
	}
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		return slices.ContainsFunc(joined.Unwrap(), isInteractiveCancellation)
	}
	for current := err; current != nil; current = errors.Unwrap(current) {
		if current.Error() == queryCancelled {
			return true
		}
	}
	return false
}

func newVersionCmd(buildVersion string) *cobra.Command {
	var updateProtocol, updateBuild bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: locale.Text(locale.KeyQueryShowZevVersion),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if updateBuild {
				return writeQueryOutput(cmd.OutOrStdout(), "%s\n", buildVersion)
			}
			if updateProtocol {
				return writeQueryOutput(cmd.OutOrStdout(), "zev-update-v1\n")
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), locale.Text(locale.KeyQueryZevVersionMessage), buildVersion); err != nil {
				return fmt.Errorf("write version: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&updateProtocol, "update-protocol", false, locale.Text(locale.KeyUpdateProtocol))
	cmd.Flags().Lookup("update-protocol").Hidden = true
	cmd.Flags().BoolVar(&updateBuild, "update-build", false, locale.Text(locale.KeyUpdateProtocol))
	cmd.Flags().Lookup("update-build").Hidden = true
	return cmd
}
