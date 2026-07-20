package secretsmanager

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssm "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/sops-aws-sync/internal/domain"
)

// fakeClient records consumed AWS requests and returns configured responses.
type fakeClient struct {
	describeOutput *awssm.DescribeSecretOutput
	describeErr    error
	getOutput      *awssm.GetSecretValueOutput
	getErr         error
	createInput    *awssm.CreateSecretInput
	createErr      error
	putInput       *awssm.PutSecretValueInput
	putErr         error
}

// DescribeSecret returns configured direct metadata evidence.
func (client *fakeClient) DescribeSecret(
	_ context.Context,
	_ *awssm.DescribeSecretInput,
	_ ...func(*awssm.Options),
) (*awssm.DescribeSecretOutput, error) {
	return client.describeOutput, client.describeErr
}

// GetSecretValue returns the configured AWSCURRENT payload.
func (client *fakeClient) GetSecretValue(
	_ context.Context,
	_ *awssm.GetSecretValueInput,
	_ ...func(*awssm.Options),
) (*awssm.GetSecretValueOutput, error) {
	return client.getOutput, client.getErr
}

// CreateSecret records a canonical create request.
func (client *fakeClient) CreateSecret(
	_ context.Context,
	input *awssm.CreateSecretInput,
	_ ...func(*awssm.Options),
) (*awssm.CreateSecretOutput, error) {
	client.createInput = input

	return &awssm.CreateSecretOutput{}, client.createErr
}

// PutSecretValue records a canonical update request.
func (client *fakeClient) PutSecretValue(
	_ context.Context,
	input *awssm.PutSecretValueInput,
	_ ...func(*awssm.Options),
) (*awssm.PutSecretValueOutput, error) {
	client.putInput = input

	return &awssm.PutSecretValueOutput{}, client.putErr
}

// TestObserveNormalizesDirectOwnedString proves exact tags and AWSCURRENT evidence translation.
func TestObserveNormalizesDirectOwnedString(t *testing.T) {
	t.Parallel()

	desired, scope := adapterDesired(t)
	client := &fakeClient{
		describeOutput: &awssm.DescribeSecretOutput{
			Tags: []types.Tag{
				{Key: aws.String(domain.ManagedByTagKey), Value: aws.String(domain.ManagedByTagValue)},
				{Key: aws.String(domain.ScopeTagKey), Value: aws.String(scope.Value())},
				{Key: aws.String(domain.SourceTagKey), Value: aws.String(desired.Source().Value())},
				{Key: aws.String("unreserved"), Value: aws.String("preserved")},
			},
			VersionIdsToStages: map[string][]string{"version-1": {awsCurrent}},
		},
		getOutput: &awssm.GetSecretValueOutput{
			VersionId: aws.String("version-1"), VersionStages: []string{awsCurrent},
			SecretString: aws.String(`{"password":"sentinel"}`),
		},
	}
	adapter, err := New(client, time.Second)
	require.NoError(t, err)

	evidence, err := adapter.Observe(context.Background(), desired.Name())
	require.NoError(t, err)
	assert.True(t, evidence.Exists)
	assert.True(t, evidence.StagingValid)
	assert.Equal(t, "version-1", evidence.CurrentVersion)
	assert.Len(t, evidence.ReservedTags, 3)
	assert.Equal(t, domain.ObservedOwnedActiveString, domain.ClassifyDirect(desired, scope, evidence).Kind())
}

// TestCreateAndUpdateUseCanonicalValuesTokensAndReservedTags proves mutation request shape.
func TestCreateAndUpdateUseCanonicalValuesTokensAndReservedTags(t *testing.T) {
	t.Parallel()

	desired, scope := adapterDesired(t)
	client := &fakeClient{}
	adapter, err := New(client, time.Second)
	require.NoError(t, err)

	require.NoError(t, adapter.Create(context.Background(), desired, scope, "0123456789abcdef0123456789abcdef"))
	require.NotNil(t, client.createInput)
	assert.Equal(t, desired.Name().Value(), aws.ToString(client.createInput.Name))
	assert.Equal(t, string(desired.Value().CopyCanonicalJSON()), aws.ToString(client.createInput.SecretString))
	assert.Equal(t, "0123456789abcdef0123456789abcdef", aws.ToString(client.createInput.ClientRequestToken))
	assert.ElementsMatch(t, []types.Tag{
		{Key: aws.String(domain.ManagedByTagKey), Value: aws.String(domain.ManagedByTagValue)},
		{Key: aws.String(domain.ScopeTagKey), Value: aws.String(scope.Value())},
		{Key: aws.String(domain.SourceTagKey), Value: aws.String(desired.Source().Value())},
	}, client.createInput.Tags)

	require.NoError(t, adapter.Update(context.Background(), desired, "fedcba9876543210fedcba9876543210"))
	require.NotNil(t, client.putInput)
	assert.Equal(t, desired.Name().Value(), aws.ToString(client.putInput.SecretId))
	assert.Equal(t, string(desired.Value().CopyCanonicalJSON()), aws.ToString(client.putInput.SecretString))
	assert.Equal(t, []string{awsCurrent}, client.putInput.VersionStages)
}

// TestMutationTimeoutIsTypedAmbiguous proves lost-response evidence is never a raw error string.
func TestMutationTimeoutIsTypedAmbiguous(t *testing.T) {
	t.Parallel()

	desired, scope := adapterDesired(t)
	client := &fakeClient{createErr: context.DeadlineExceeded}
	adapter, err := New(client, time.Second)
	require.NoError(t, err)

	err = adapter.Create(context.Background(), desired, scope, "0123456789abcdef0123456789abcdef")
	var safe *SafeError
	require.ErrorAs(t, err, &safe)
	assert.True(t, safe.Ambiguous())
	assert.Equal(t, "AWS create failed", safe.Error())
	assert.NotContains(t, safe.Error(), context.DeadlineExceeded.Error())
}

// adapterDesired constructs one owned test secret and scope.
func adapterDesired(t *testing.T) (domain.DesiredSecret, domain.ScopeIdentity) {
	t.Helper()
	name, err := domain.NewSecretName("/acme/payments/database")
	require.NoError(t, err)
	source, err := domain.NewSourceIdentity("secrets/database.sops.json")
	require.NoError(t, err)
	value, err := domain.NewSecretValue([]byte(`{"password":"sentinel"}`))
	require.NoError(t, err)
	revision, err := domain.NewRevision("0123456789abcdef0123456789abcdef01234567")
	require.NoError(t, err)
	scope, err := domain.NewScopeIdentity("meigma/example", "secrets", "/acme/payments")
	require.NoError(t, err)

	return domain.NewDesiredSecret(name, source, value, revision), scope
}
