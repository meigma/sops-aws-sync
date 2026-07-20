// Package main is the sops-aws-sync process composition root.
package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/meigma/sops-aws-sync/internal/adapters/gitrepo"
	"github.com/meigma/sops-aws-sync/internal/adapters/logging"
	"github.com/meigma/sops-aws-sync/internal/adapters/secretsmanager"
	"github.com/meigma/sops-aws-sync/internal/adapters/sopsdecrypt"
	"github.com/meigma/sops-aws-sync/internal/application"
	"github.com/meigma/sops-aws-sync/internal/cli"
	"github.com/meigma/sops-aws-sync/internal/config"
	"github.com/meigma/sops-aws-sync/internal/domain"
)

//nolint:gochecknoglobals // GoReleaser injects immutable build metadata through ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// runtimeRunner is the sole production dependency composition adapter.
type runtimeRunner struct {
	newSource    func(string, int64) (application.SourceRepository, error)
	newDecrypter func(int) (application.JSONDecrypter, error)
	loadSecrets  func(context.Context, secretsmanager.LoadOptions) (application.SecretsManager, error)
}

// setupStage identifies which side-effect boundary failed during composition.
type setupStage string

const (
	setupDesired setupStage = "desired"
	setupAWS     setupStage = "aws"
)

// setupError retains the failed composition stage without exposing its cause.
type setupError struct {
	stage setupStage
	cause error
}

// Error returns a stable message that cannot disclose local or credential details.
func (failure *setupError) Error() string {
	return "runtime setup failed"
}

// Unwrap retains cancellation and typed configuration evidence.
func (failure *setupError) Unwrap() error {
	return failure.cause
}

// Plan composes and executes the read-only application use case.
func (runner runtimeRunner) Plan(
	ctx context.Context,
	runtime config.Config,
	logger *slog.Logger,
) (application.Report, error) {
	started := time.Now()
	service, revision, err := runner.newService(ctx, runtime, logger)
	if err != nil {
		return setupFailure(ctx, revision, started, err)
	}

	return service.Plan(ctx, reconcileInput(runtime))
}

// Sync composes and executes the mutating application use case.
func (runner runtimeRunner) Sync(
	ctx context.Context,
	runtime config.Config,
	logger *slog.Logger,
) (application.Report, error) {
	started := time.Now()
	service, revision, err := runner.newService(ctx, runtime, logger)
	if err != nil {
		return setupFailure(ctx, revision, started, err)
	}

	return service.Sync(ctx, reconcileInput(runtime))
}

// newService wires committed Git, SOPS, AWS, tokens, and the application service.
func (runner runtimeRunner) newService(
	ctx context.Context,
	runtime config.Config,
	logger *slog.Logger,
) (*application.Service, domain.Revision, error) {
	runner = runner.withDefaults()
	source, err := runner.newSource(runtime.Repository, int64(runtime.MaxEncryptedBytes))
	if err != nil {
		return nil, domain.Revision{}, &setupError{stage: setupDesired, cause: err}
	}
	decrypter, err := runner.newDecrypter(runtime.MaxEncryptedBytes)
	if err != nil {
		return nil, domain.Revision{}, &setupError{stage: setupDesired, cause: err}
	}
	desired, err := application.BuildDesiredSnapshot(ctx, source, decrypter, application.DesiredInput{
		Revision: runtime.Revision, SourceRoot: runtime.SourceRoot, SecretPrefix: runtime.SecretPrefix,
	})
	if err != nil {
		return nil, domain.Revision{}, &setupError{stage: setupDesired, cause: err}
	}
	secrets, err := runner.loadSecrets(ctx, secretsmanager.LoadOptions{
		Region: runtime.AWSRegion, Profile: runtime.AWSProfile,
		MaxAttempts: runtime.AWSMaxAttempts, MaxBackoff: runtime.AWSMaxBackoff,
		OperationTimeout: runtime.OperationTimeout,
	})
	if err != nil {
		return nil, desired.Revision(), &setupError{stage: setupAWS, cause: err}
	}
	service, err := application.NewService(
		desired, secrets, application.RandomTokenSource{}, logger, version,
	)
	if err != nil {
		return nil, desired.Revision(), &setupError{stage: setupDesired, cause: err}
	}

	return service, desired.Revision(), nil
}

// withDefaults supplies production adapters while preserving test injection points.
func (runner runtimeRunner) withDefaults() runtimeRunner {
	if runner.newSource == nil {
		runner.newSource = func(repository string, maximumBytes int64) (application.SourceRepository, error) {
			return gitrepo.NewReader(repository, maximumBytes)
		}
	}
	if runner.newDecrypter == nil {
		runner.newDecrypter = func(maximumBytes int) (application.JSONDecrypter, error) {
			return sopsdecrypt.New(maximumBytes)
		}
	}
	if runner.loadSecrets == nil {
		runner.loadSecrets = func(
			ctx context.Context,
			options secretsmanager.LoadOptions,
		) (application.SecretsManager, error) {
			return secretsmanager.Load(ctx, options)
		}
	}

	return runner
}

// setupFailure maps safe setup evidence to the stable report and exit contract.
func setupFailure(
	ctx context.Context,
	revision domain.Revision,
	started time.Time,
	err error,
) (application.Report, error) {
	kind := classifySetupFailure(ctx, err)
	report := application.Report{
		SchemaVersion: "sops-aws-sync/report/v1", ToolVersion: version, GitRevision: revision.Value(),
		Status: string(kind), Verification: "not-run", DurationMilliseconds: time.Since(started).Milliseconds(),
	}

	return report, application.NewOutcomeError(kind)
}

// classifySetupFailure preserves interruption and separates local/configuration input from AWS availability.
func classifySetupFailure(ctx context.Context, err error) application.OutcomeKind {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return application.OutcomeInterrupted
	}
	var staged *setupError
	if errors.As(err, &staged) && staged.stage == setupDesired {
		return application.OutcomeInvalid
	}
	var configuration interface {
		ConfigurationFailure() bool
	}
	if errors.As(err, &configuration) && configuration.ConfigurationFailure() {
		return application.OutcomeInvalid
	}

	return application.OutcomeApplyFailed
}

// reconcileInput maps validated CLI configuration to the application contract.
func reconcileInput(runtime config.Config) application.ReconcileInput {
	return application.ReconcileInput{
		RepositoryID: runtime.RepositoryID, SourceRoot: runtime.SourceRoot,
		SecretPrefix: runtime.SecretPrefix, VerificationTimeout: runtime.VerificationTimeout,
		ShowResourceNames: runtime.ShowResourceNames,
	}
}

// main exits with the stable status returned by run.
func main() {
	os.Exit(run())
}

// run owns signal handling, SOPS log suppression, streams, and command execution.
func run() int {
	logrus.SetOutput(io.Discard)
	logrus.SetLevel(logrus.PanicLevel)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	root := cli.NewRootCommand(cli.Options{
		In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Runner: runtimeRunner{}, NewLogger: logging.New,
		Build: cli.BuildInfo{Version: version, Commit: commit, Date: date},
	})
	err := root.ExecuteContext(ctx)

	return cli.ExitCode(err)
}
