package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/sops-aws-sync/internal/adapters/secretsmanager"
	"github.com/meigma/sops-aws-sync/internal/application"
	"github.com/meigma/sops-aws-sync/internal/config"
	"github.com/meigma/sops-aws-sync/internal/domain"
)

// setupSource records committed source loading during runtime composition.
type setupSource struct {
	events *[]string
}

// Load returns one valid committed encrypted document.
func (source *setupSource) Load(
	_ context.Context,
	_, _ string,
) (application.SourceSnapshot, error) {
	*source.events = append(*source.events, "source")
	revision, err := domain.NewRevision("0123456789abcdef0123456789abcdef01234567")
	if err != nil {
		return application.SourceSnapshot{}, err
	}

	return application.SourceSnapshot{
		Revision: revision,
		Documents: []application.EncryptedDocument{
			{Path: "secrets/value.sops.json", Format: domain.SourceFormatJSON, Data: []byte("encrypted")},
		},
	}, nil
}

// setupDecrypter records decryption and optionally rejects desired input.
type setupDecrypter struct {
	events *[]string
	err    error
}

// configurationSetupError marks a credential setup failure as operator configuration.
type configurationSetupError struct{}

// Error returns a safe test failure string.
func (configurationSetupError) Error() string {
	return "configuration failed"
}

// ConfigurationFailure reports that operator input caused the setup failure.
func (configurationSetupError) ConfigurationFailure() bool {
	return true
}

// Decrypt returns one canonical value unless configured to fail.
func (decrypter *setupDecrypter) Decrypt(
	_ context.Context,
	_ domain.SourceFormat,
	_ []byte,
) (domain.SecretValue, error) {
	*decrypter.events = append(*decrypter.events, "decrypt")
	if decrypter.err != nil {
		return domain.SecretValue{}, decrypter.err
	}

	return domain.NewSecretValue([]byte(`{"value":"desired"}`))
}

// TestRuntimeBuildsDesiredStateBeforeAWS proves invalid local input cannot start credential loading.
func TestRuntimeBuildsDesiredStateBeforeAWS(t *testing.T) {
	t.Parallel()

	events := []string{}
	runner := setupRunner(&events, errors.New("decryption sentinel"), errors.New("AWS must not load"))
	report, err := runner.Sync(context.Background(), setupConfig(), slog.Default())
	var outcome *application.OutcomeError
	require.ErrorAs(t, err, &outcome)
	assert.Equal(t, application.OutcomeInvalid, outcome.Kind())
	assert.Equal(t, application.ReportSchemaVersion, report.SchemaVersion)
	assert.Equal(t, "invalid", report.Status)
	assert.Equal(t, []string{"source", "decrypt"}, events)
}

// TestRuntimePreservesAWSSetupFailureClassifications proves cancellation and provider failure stay distinct.
func TestRuntimePreservesAWSSetupFailureClassifications(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		loadErr  error
		wantKind application.OutcomeKind
	}{
		{name: "cancellation", loadErr: context.Canceled, wantKind: application.OutcomeInterrupted},
		{name: "configuration", loadErr: configurationSetupError{}, wantKind: application.OutcomeInvalid},
		{
			name:     "provider failure",
			loadErr:  errors.New("provider unavailable"),
			wantKind: application.OutcomeApplyFailed,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			events := []string{}
			runner := setupRunner(&events, nil, test.loadErr)
			report, err := runner.Plan(context.Background(), setupConfig(), slog.Default())
			var outcome *application.OutcomeError
			require.ErrorAs(t, err, &outcome)
			assert.Equal(t, test.wantKind, outcome.Kind())
			assert.Equal(t, application.ReportSchemaVersion, report.SchemaVersion)
			assert.Equal(t, string(test.wantKind), report.Status)
			assert.Equal(t, []string{"source", "decrypt", "aws"}, events)
			assert.Equal(t, "0123456789abcdef0123456789abcdef01234567", report.GitRevision)
		})
	}
}

// setupRunner builds an injected composition root with observable setup ordering.
func setupRunner(events *[]string, decryptErr, loadErr error) runtimeRunner {
	return runtimeRunner{
		newSource: func(_ string, _ int64) (application.SourceRepository, error) {
			return &setupSource{events: events}, nil
		},
		newDecrypter: func(_ int) (application.DocumentDecrypter, error) {
			return &setupDecrypter{events: events, err: decryptErr}, nil
		},
		loadSecrets: func(
			_ context.Context,
			_ secretsmanager.LoadOptions,
		) (application.SecretsManager, error) {
			*events = append(*events, "aws")

			return nil, loadErr
		},
	}
}

// setupConfig returns the minimum valid runtime settings consumed by composition.
func setupConfig() config.Config {
	return config.Config{
		Repository: ".", Revision: "HEAD", RepositoryID: "meigma/example", SourceRoot: "secrets",
		SecretPrefix: "/acme/example", MaxEncryptedBytes: 1024, OperationTimeout: time.Second,
		VerificationTimeout: time.Second,
	}
}
