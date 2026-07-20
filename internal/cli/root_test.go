package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/sops-aws-sync/internal/application"
	"github.com/meigma/sops-aws-sync/internal/config"
	"github.com/meigma/sops-aws-sync/internal/domain"
)

// recordingRunner captures validated configuration and returns configured outcomes.
type recordingRunner struct {
	runtime    config.Config
	planReport application.Report
	planErr    error
	syncReport application.Report
	syncErr    error
	calls      int
}

// Plan records one read-only invocation.
func (runner *recordingRunner) Plan(
	_ context.Context,
	runtime config.Config,
	_ *slog.Logger,
) (application.Report, error) {
	runner.runtime = runtime
	runner.calls++

	return runner.planReport, runner.planErr
}

// Sync records one mutating invocation.
func (runner *recordingRunner) Sync(
	_ context.Context,
	runtime config.Config,
	_ *slog.Logger,
) (application.Report, error) {
	runner.runtime = runtime
	runner.calls++

	return runner.syncReport, runner.syncErr
}

// TestVersionContractsDoNotLoadConfiguration proves both version surfaces are dependency-free.
func TestVersionContractsDoNotLoadConfiguration(t *testing.T) {
	t.Parallel()

	for _, arguments := range [][]string{{"--version"}, {"version"}} {
		var stdout bytes.Buffer
		root := NewRootCommand(Options{
			Out:   &stdout,
			Build: BuildInfo{Version: "0.1.0", Commit: "abc1234", Date: "2026-05-08T10:00:00Z"},
		})
		root.SetArgs(arguments)
		require.NoError(t, root.ExecuteContext(context.Background()))
		assert.Equal(t, "sops-aws-sync 0.1.0 (abc1234) built 2026-05-08T10:00:00Z\n", stdout.String())
	}
}

// TestRootWithoutSubcommandShowsHelpWithoutConfiguration proves the command family entrypoint is discoverable.
func TestRootWithoutSubcommandShowsHelpWithoutConfiguration(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	root := NewRootCommand(Options{Out: &stdout})
	require.NoError(t, root.ExecuteContext(context.Background()))
	assert.Contains(t, stdout.String(), "Available Commands")
}

// TestConfigurationPrecedenceUsesFlagsThenEnvironmentThenExplicitFile proves typed Viper translation.
func TestConfigurationPrecedenceUsesFlagsThenEnvironmentThenExplicitFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(
		t,
		os.WriteFile(configPath, []byte("repository-id: file/repository\nsecret-prefix: /file/prefix\n"), 0o600),
	)
	t.Setenv("SOPS_AWS_SYNC_SECRET_PREFIX", "/environment/prefix")
	runner := &recordingRunner{planReport: successfulReport()}
	root := NewRootCommand(Options{Out: ioBuffer(), Viper: viper.New(), Runner: runner, NewLogger: testLogger})
	root.SetArgs([]string{
		"plan", "--config", configPath, "--repository-id", "flag/repository", "--log-format", "text",
	})

	require.NoError(t, root.ExecuteContext(context.Background()))
	assert.Equal(t, "flag/repository", runner.runtime.RepositoryID)
	assert.Equal(t, "/environment/prefix", runner.runtime.SecretPrefix)
}

// TestUnknownConfigurationKeyFailsBeforeRunner proves malformed files have no side effects.
func TestUnknownConfigurationKeyFailsBeforeRunner(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath,
		[]byte("repository-id: meigma/example\nsecret-prefix: /acme/payments\nunknown-key: true\n"), 0o600))
	runner := &recordingRunner{planReport: successfulReport()}
	var stderr bytes.Buffer
	root := NewRootCommand(Options{
		Out: ioBuffer(), Err: &stderr, Viper: viper.New(), Runner: runner, NewLogger: testLogger,
	})
	root.SetArgs([]string{"plan", "--config", configPath})

	err := root.ExecuteContext(context.Background())
	assert.Equal(t, 3, ExitCode(err))
	assert.Zero(t, runner.calls)
	assert.Contains(t, stderr.String(), "configuration contains unknown keys or invalid value types")
}

// TestInvalidConfigurationWritesSafeDiagnostic proves exit 3 remains actionable and non-disclosing.
func TestInvalidConfigurationWritesSafeDiagnostic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		arguments []string
		want      string
	}{
		{
			name:      "missing required ownership flags",
			arguments: []string{"plan"},
			want:      "configuration error: repository-id is required",
		},
		{
			name: "invalid duration",
			arguments: []string{
				"plan", "--repository-id", "meigma/example", "--secret-prefix", "/acme/payments",
				"--operation-timeout", "phase2-secret-sentinel",
			},
			want: "configuration error: operation-timeout is invalid",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			root := NewRootCommand(Options{
				Out: &stdout, Err: &stderr, Viper: viper.New(), Runner: &recordingRunner{}, NewLogger: testLogger,
			})
			root.SetArgs(test.arguments)

			err := root.ExecuteContext(context.Background())
			assert.Equal(t, 3, ExitCode(err))
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), test.want)
			assert.NotContains(t, stderr.String(), "phase2-secret-sentinel")
		})
	}
}

// TestDetailedPlanExitAndReportFile prove drift status 2 and safe machine persistence.
func TestDetailedPlanExitAndReportFile(t *testing.T) {
	t.Parallel()

	reportPath := filepath.Join(t.TempDir(), "report.json")
	report := successfulReport()
	report.Status = "drift"
	report.Counts.Create = 1
	runner := &recordingRunner{planReport: report}
	root := NewRootCommand(Options{Out: ioBuffer(), Viper: viper.New(), Runner: runner, NewLogger: testLogger})
	root.SetArgs([]string{
		"plan", "--repository-id", "meigma/example", "--secret-prefix", "/acme/payments",
		"--report-file", reportPath, "--detailed-exit-code",
	})

	err := root.ExecuteContext(context.Background())
	assert.Equal(t, 2, ExitCode(err))
	encoded, readErr := os.ReadFile(reportPath)
	require.NoError(t, readErr)
	var persisted application.Report
	require.NoError(t, json.Unmarshal(encoded, &persisted))
	assert.Equal(t, report.Status, persisted.Status)
	assert.NotContains(t, string(encoded), "/acme/payments")
}

// TestApplicationOutcomeMapsToStableExitCode proves application classifications retain public statuses.
func TestApplicationOutcomeMapsToStableExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind     application.OutcomeKind
		exitCode int
	}{
		{kind: application.OutcomeConflict, exitCode: 4},
		{kind: application.OutcomeApplyFailed, exitCode: 5},
		{kind: application.OutcomeInterrupted, exitCode: 130},
	}
	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			t.Parallel()

			report := successfulReport()
			report.Status = string(test.kind)
			runner := &recordingRunner{
				syncReport: report,
				syncErr:    application.NewOutcomeError(test.kind),
			}
			root := NewRootCommand(Options{
				Out: ioBuffer(), Viper: viper.New(), Runner: runner, NewLogger: testLogger,
			})
			root.SetArgs([]string{
				"sync", "--repository-id", "meigma/example", "--secret-prefix", "/acme/payments",
			})

			err := root.ExecuteContext(context.Background())
			assert.Equal(t, test.exitCode, ExitCode(err))
		})
	}
}

// successfulReport returns one converged secret-free test report.
func successfulReport() application.Report {
	return application.Report{
		SchemaVersion: application.ReportSchemaVersion, ToolVersion: "test",
		GitRevision: "0123456789abcdef0123456789abcdef01234567", Status: "converged",
		Counts: domain.Counts{Unchanged: 1}, Verification: "converged",
	}
}

// ioBuffer returns a discardable in-memory stream.
func ioBuffer() *bytes.Buffer {
	return &bytes.Buffer{}
}

// testLogger constructs a deterministic JSON logger for command tests.
func testLogger(output io.Writer, _, _ string) (*slog.Logger, error) {
	return slog.New(slog.NewJSONHandler(output, nil)), nil
}
