package application_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awssm "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	secretsadapter "github.com/meigma/sops-aws-sync/internal/adapters/secretsmanager"
	"github.com/meigma/sops-aws-sync/internal/application"
	"github.com/meigma/sops-aws-sync/internal/domain"
)

// TestAWSSandboxCreateUpdateNoOpAndVerification proves the opt-in genuine service slice.
func TestAWSSandboxCreateUpdateNoOpAndVerification(t *testing.T) {
	prefix := strings.TrimSuffix(os.Getenv("SOPS_AWS_SYNC_AWS_SANDBOX_PREFIX"), "/")
	region := os.Getenv("SOPS_AWS_SYNC_AWS_SANDBOX_REGION")
	if prefix == "" || region == "" {
		t.Skip("set SOPS_AWS_SYNC_AWS_SANDBOX_PREFIX and SOPS_AWS_SYNC_AWS_SANDBOX_REGION")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	awsConfiguration, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	require.NoError(t, err)
	client := awssm.NewFromConfig(awsConfiguration)
	adapter, err := secretsadapter.New(client, 30*time.Second)
	require.NoError(t, err)
	nameSuffix := sandboxToken(t)
	secretName, err := domain.NewSecretName(prefix + "/phase2-" + nameSuffix)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, _ = client.DeleteSecret(cleanupContext, &awssm.DeleteSecretInput{
			SecretId: aws.String(secretName.Value()), RecoveryWindowInDays: aws.Int64(7),
		})
	})

	revision, err := domain.NewRevision("0123456789abcdef0123456789abcdef01234567")
	require.NoError(t, err)
	initialValue, err := domain.NewSecretValue([]byte(`{"value":"initial"}`))
	require.NoError(t, err)
	source := &fakeSource{snapshot: application.SourceSnapshot{
		Revision: revision,
		Documents: []application.EncryptedDocument{
			{Path: "secrets/phase2-" + nameSuffix + ".sops.json", Data: []byte("encrypted")},
		},
	}}
	decrypter := &fakeDecrypter{value: initialValue}
	service, err := application.NewService(
		source,
		decrypter,
		adapter,
		application.RandomTokenSource{},
		slog.Default(),
		"sandbox",
	)
	require.NoError(t, err)
	input := application.ReconcileInput{
		RepositoryID: "meigma/sops-aws-sync-sandbox", Revision: "HEAD", SourceRoot: "secrets",
		SecretPrefix: prefix, VerificationTimeout: 30 * time.Second,
	}

	report, err := service.Sync(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Counts.Create)
	report, err = service.Sync(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Counts.Unchanged)
	_, err = client.PutSecretValue(ctx, &awssm.PutSecretValueInput{
		SecretId: aws.String(secretName.Value()), SecretString: aws.String(`{"value":"external-drift"}`),
		ClientRequestToken: aws.String(sandboxToken(t)),
	})
	require.NoError(t, err)
	report, err = service.Sync(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Counts.Update)
	report, err = service.Sync(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Counts.Unchanged)
}

// sandboxToken returns a random 32-character token suitable for names and AWS writes.
func sandboxToken(t *testing.T) string {
	t.Helper()
	var raw [16]byte
	_, err := rand.Read(raw[:])
	require.NoError(t, err)

	return hex.EncodeToString(raw[:])
}
