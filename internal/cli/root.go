package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/meigma/sops-aws-sync/internal/application"
	"github.com/meigma/sops-aws-sync/internal/config"
)

const (
	commandPlan               = "plan"
	commandVersion            = "version"
	exitSuccess               = 0
	exitDrift                 = 2
	exitInvalid               = 3
	exitConflict              = 4
	exitApplyFailed           = 5
	exitVerification          = 6
	exitInterrupted           = 130
	defaultRecoveryWindowDays = 30
	reportFileMode            = 0o600
)

// BuildInfo describes linker-injected build metadata.
type BuildInfo struct {
	// Version is the release version.
	Version string
	// Commit is the source commit used to build the binary.
	Commit string
	// Date is the build timestamp.
	Date string
}

// Runner composes infrastructure for the plan and sync application use cases.
type Runner interface {
	Plan(ctx context.Context, runtime config.Config, logger *slog.Logger) (application.Report, error)
	Sync(ctx context.Context, runtime config.Config, logger *slog.Logger) (application.Report, error)
}

// LoggerFactory constructs the injected production logger after config validation.
type LoggerFactory func(output io.Writer, level, format string) (*slog.Logger, error)

// Options customizes root command construction.
type Options struct {
	// In receives command input.
	In io.Reader
	// Out receives production operational logs and version output.
	Out io.Writer
	// Err receives Cobra usage and emergency initialization diagnostics.
	Err io.Writer
	// Build controls version output and report metadata.
	Build BuildInfo
	// Viper is the instance-local configuration provider.
	Viper *viper.Viper
	// Runner executes plan and sync after configuration validation.
	Runner Runner
	// NewLogger constructs the injected logger on the CLI-owned stdout stream.
	NewLogger LoggerFactory
}

// commandError carries one stable process exit status.
type commandError struct {
	code int
}

// Error returns a fixed non-sensitive failure string.
func (failure *commandError) Error() string {
	return "command failed"
}

// ExitCode maps a command error to the stable CLI compatibility code.
func ExitCode(err error) int {
	if err == nil {
		return exitSuccess
	}
	var failure *commandError
	if errors.As(err, &failure) {
		return failure.code
	}

	return exitInvalid
}

// NewRootCommand creates the sops-aws-sync Cobra command tree.
func NewRootCommand(options Options) *cobra.Command {
	options = options.withDefaults()
	var configPath string
	root := &cobra.Command{
		Use:           "sops-aws-sync",
		Short:         "Reconcile committed SOPS JSON with AWS Secrets Manager",
		Version:       options.Build.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		PersistentPreRunE: func(command *cobra.Command, _ []string) error {
			if command == command.Root() || command.Name() == commandVersion {
				return nil
			}
			if err := initializeConfig(command, options.Viper, configPath); err != nil {
				writeConfigurationDiagnostic(command.ErrOrStderr(), err)
				return &commandError{code: exitInvalid}
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	root.SetVersionTemplate(versionLine(options.Build))
	root.SetIn(options.In)
	root.SetOut(options.Out)
	root.SetErr(options.Err)
	root.SetFlagErrorFunc(func(command *cobra.Command, _ error) error {
		_, _ = io.WriteString(command.ErrOrStderr(), "invalid command-line flags; use --help for supported values\n")
		return &commandError{code: exitInvalid}
	})
	addRuntimeFlags(root.PersistentFlags(), &configPath)
	root.AddCommand(newPlanCommand(options), newSyncCommand(options), newVersionCommand(options.Build))

	return root
}

// withDefaults fills non-sensitive command-construction defaults.
func (options Options) withDefaults() Options {
	if options.In == nil {
		options.In = strings.NewReader("")
	}
	if options.Out == nil {
		options.Out = io.Discard
	}
	if options.Err == nil {
		options.Err = io.Discard
	}
	if options.Viper == nil {
		options.Viper = viper.New()
	}
	options.Build = options.Build.withDefaults()

	return options
}

// withDefaults fills missing linker metadata.
func (build BuildInfo) withDefaults() BuildInfo {
	if strings.TrimSpace(build.Version) == "" {
		build.Version = "dev"
	}
	if strings.TrimSpace(build.Commit) == "" {
		build.Commit = "none"
	}
	if strings.TrimSpace(build.Date) == "" {
		build.Date = "unknown"
	}

	return build
}

// versionLine returns the stable human-readable build metadata line.
func versionLine(build BuildInfo) string {
	return fmt.Sprintf("sops-aws-sync %s (%s) built %s\n", build.Version, build.Commit, build.Date)
}

// newVersionCommand creates the configuration-free version subcommand.
func newVersionCommand(build BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   commandVersion,
		Short: "Print release, commit, and build date",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, err := io.WriteString(command.OutOrStdout(), versionLine(build))
			if err != nil {
				return &commandError{code: exitInvalid}
			}
			return nil
		},
	}
}

// newPlanCommand creates the read-only desired-name planning command.
func newPlanCommand(options Options) *cobra.Command {
	var detailedExitCode bool
	command := &cobra.Command{
		Use:   commandPlan,
		Short: "Build a redacted reconciliation plan without mutation",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return executeUseCase(command, options, false, detailedExitCode)
		},
	}
	command.Flags().
		BoolVar(&detailedExitCode, "detailed-exit-code", false, "return exit code 2 when executable drift exists")

	return command
}

// newSyncCommand creates the mutating and verifying synchronization command.
func newSyncCommand(options Options) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Apply the plan and verify direct desired-name convergence",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return executeUseCase(command, options, true, false)
		},
	}
}

// executeUseCase validates typed configuration, runs one use case, and persists its report.
func executeUseCase(command *cobra.Command, options Options, syncMode, detailedExitCode bool) error {
	runtime, err := config.Load(options.Viper)
	if err != nil {
		writeConfigurationDiagnostic(command.ErrOrStderr(), err)
		return &commandError{code: exitInvalid}
	}
	if options.Runner == nil {
		_, _ = io.WriteString(command.ErrOrStderr(), "command execution is unavailable\n")
		return &commandError{code: exitInvalid}
	}
	if options.NewLogger == nil {
		_, _ = io.WriteString(command.ErrOrStderr(), "logging initialization is unavailable\n")
		return &commandError{code: exitInvalid}
	}
	logger, err := options.NewLogger(options.Out, runtime.LogLevel, runtime.LogFormat)
	if err != nil {
		_, _ = io.WriteString(options.Err, "failed to initialize logging\n")
		return &commandError{code: exitInvalid}
	}
	ctx, cancel := context.WithTimeout(command.Context(), runtime.RunTimeout)
	defer cancel()
	var report application.Report
	if syncMode {
		report, err = options.Runner.Sync(ctx, runtime, logger)
	} else {
		report, err = options.Runner.Plan(ctx, runtime, logger)
	}
	if writeErr := writeReport(runtime.ReportFile, report); writeErr != nil {
		logger.ErrorContext(ctx, "report persistence failed", "phase", "report", "status", "failed")
		return &commandError{code: exitInvalid}
	}
	if err != nil {
		code := outcomeExitCode(err)
		logger.ErrorContext(ctx, "command failed", "phase", "complete", "status", report.Status)
		return &commandError{code: code}
	}
	counts := report.Counts
	if !syncMode && detailedExitCode && counts.Create+counts.Update+counts.Restore+counts.ScheduleDelete > 0 {
		return &commandError{code: exitDrift}
	}

	return nil
}

// outcomeExitCode maps application outcomes to stable public process statuses.
func outcomeExitCode(err error) int {
	var outcome *application.OutcomeError
	if !errors.As(err, &outcome) {
		return exitApplyFailed
	}
	switch outcome.Kind() {
	case application.OutcomeInvalid:
		return exitInvalid
	case application.OutcomeConflict:
		return exitConflict
	case application.OutcomeApplyFailed:
		return exitApplyFailed
	case application.OutcomeVerification:
		return exitVerification
	case application.OutcomeInterrupted:
		return exitInterrupted
	}

	return exitApplyFailed
}

// initializeConfig establishes flags, explicit environment, optional file, and defaults precedence.
func initializeConfig(command *cobra.Command, vp *viper.Viper, configPath string) error {
	config.SetDefaults(vp)
	if err := config.BindEnvironment(vp); err != nil {
		return err
	}
	if err := bindRuntimeFlags(command.Root().PersistentFlags(), vp); err != nil {
		return err
	}
	if configPath != "" {
		vp.SetConfigFile(configPath)
		if err := vp.ReadInConfig(); err != nil {
			return errors.New("configuration file could not be read")
		}
	}
	_, err := config.Load(vp)

	return err
}

// writeConfigurationDiagnostic emits a safe validation reason without rejected values.
func writeConfigurationDiagnostic(output io.Writer, err error) {
	_, _ = fmt.Fprintf(output, "configuration error: %s\n", err.Error())
}

// bindRuntimeFlags binds only stable runtime keys and excludes Cobra-only controls.
func bindRuntimeFlags(flags *pflag.FlagSet, vp *viper.Viper) error {
	keys := []string{
		"repository", "revision", "repository-id", "source-root", "secret-prefix",
		"max-encrypted-bytes", "allow-empty", "aws-region", "aws-profile",
		"recovery-window-days", "run-timeout", "operation-timeout", "verification-timeout",
		"aws-max-attempts", "aws-max-backoff", "log-level", "log-format", "show-resource-names", "report-file",
	}
	for _, key := range keys {
		flag := flags.Lookup(key)
		if flag == nil {
			return fmt.Errorf("runtime flag %s is missing", key)
		}
		if err := vp.BindPFlag(key, flag); err != nil {
			return fmt.Errorf("bind runtime flag %s: %w", key, err)
		}
	}

	return nil
}

// addRuntimeFlags defines the complete stable Phase 2 configuration surface.
func addRuntimeFlags(flags *pflag.FlagSet, configPath *string) {
	flags.StringVar(configPath, "config", "", "optional YAML configuration file")
	flags.String("repository", ".", "local Git repository")
	flags.String("revision", "HEAD", "committed Git revision")
	flags.String("repository-id", "", "stable repository ownership identity")
	flags.String("source-root", "secrets", "repository-relative SOPS source root")
	flags.String("secret-prefix", "", "AWS secret name prefix")
	flags.String("max-encrypted-bytes", "8MiB", "per-document encrypted input limit")
	flags.Bool("allow-empty", false, "authorize empty desired-state deletions")
	flags.String("aws-region", "", "optional standard-chain Region override")
	flags.String("aws-profile", "", "optional shared AWS profile")
	flags.Int("recovery-window-days", defaultRecoveryWindowDays, "scheduled deletion recovery window")
	flags.String("run-timeout", "10m", "whole command timeout")
	flags.String("operation-timeout", "30s", "per-AWS-call timeout")
	flags.String("verification-timeout", "2m", "direct verification timeout")
	flags.Int("aws-max-attempts", 0, "bounded AWS attempts override")
	flags.String("aws-max-backoff", "0s", "bounded AWS retry backoff override")
	flags.String("log-level", "info", "debug, info, warn, or error")
	flags.String("log-format", "json", "json or text")
	flags.Bool("show-resource-names", false, "include names and paths in output")
	flags.String("report-file", "", "non-sensitive machine report path")
}

// writeReport atomically persists a restrictive-permission JSON report when requested.
func writeReport(reportPath string, report application.Report) error {
	if reportPath == "" {
		return nil
	}
	directory := filepath.Dir(reportPath)
	temporary, err := os.CreateTemp(directory, ".sops-aws-sync-report-*")
	if err != nil {
		return errors.New("create report file")
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(reportFileMode); err != nil {
		_ = temporary.Close()
		return errors.New("restrict report file")
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(report); err != nil {
		_ = temporary.Close()
		return errors.New("encode report")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return errors.New("sync report")
	}
	if err := temporary.Close(); err != nil {
		return errors.New("close report")
	}
	if err := os.Rename(temporaryPath, reportPath); err != nil {
		return errors.New("install report")
	}

	return nil
}
