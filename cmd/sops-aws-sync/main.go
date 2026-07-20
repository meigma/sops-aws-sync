// Package main is the sops-aws-sync process composition root.
package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/meigma/sops-aws-sync/internal/adapters/gitrepo"
	"github.com/meigma/sops-aws-sync/internal/adapters/logging"
	"github.com/meigma/sops-aws-sync/internal/adapters/secretsmanager"
	"github.com/meigma/sops-aws-sync/internal/adapters/sopsdecrypt"
	"github.com/meigma/sops-aws-sync/internal/application"
	"github.com/meigma/sops-aws-sync/internal/cli"
	"github.com/meigma/sops-aws-sync/internal/config"
)

//nolint:gochecknoglobals // GoReleaser injects immutable build metadata through ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// runtimeRunner is the sole production dependency composition adapter.
type runtimeRunner struct{}

// Plan composes and executes the read-only application use case.
func (runtimeRunner) Plan(
	ctx context.Context,
	runtime config.Config,
	logger *slog.Logger,
) (application.Report, error) {
	service, err := newService(ctx, runtime, logger)
	if err != nil {
		return application.Report{SchemaVersion: "sops-aws-sync/report/v1", ToolVersion: version, Status: "invalid"},
			application.NewOutcomeError(application.OutcomeInvalid)
	}

	return service.Plan(ctx, reconcileInput(runtime))
}

// Sync composes and executes the mutating application use case.
func (runtimeRunner) Sync(
	ctx context.Context,
	runtime config.Config,
	logger *slog.Logger,
) (application.Report, error) {
	service, err := newService(ctx, runtime, logger)
	if err != nil {
		return application.Report{SchemaVersion: "sops-aws-sync/report/v1", ToolVersion: version, Status: "invalid"},
			application.NewOutcomeError(application.OutcomeInvalid)
	}

	return service.Sync(ctx, reconcileInput(runtime))
}

// newService wires committed Git, SOPS, AWS, tokens, and the application service.
func newService(ctx context.Context, runtime config.Config, logger *slog.Logger) (*application.Service, error) {
	source, err := gitrepo.NewReader(runtime.Repository, int64(runtime.MaxEncryptedBytes))
	if err != nil {
		return nil, err
	}
	decrypter, err := sopsdecrypt.New(runtime.MaxEncryptedBytes)
	if err != nil {
		return nil, err
	}
	secrets, err := secretsmanager.Load(ctx, secretsmanager.LoadOptions{
		Region: runtime.AWSRegion, Profile: runtime.AWSProfile,
		MaxAttempts: runtime.AWSMaxAttempts, MaxBackoff: runtime.AWSMaxBackoff,
		OperationTimeout: runtime.OperationTimeout,
	})
	if err != nil {
		return nil, err
	}

	return application.NewService(source, decrypter, secrets, application.RandomTokenSource{}, logger, version)
}

// reconcileInput maps validated CLI configuration to the application contract.
func reconcileInput(runtime config.Config) application.ReconcileInput {
	return application.ReconcileInput{
		RepositoryID: runtime.RepositoryID, Revision: runtime.Revision, SourceRoot: runtime.SourceRoot,
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
