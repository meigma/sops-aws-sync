package secretsmanager

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awssm "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/sops-aws-sync/internal/domain"
)

// fakeClient records consumed AWS requests and returns configured responses.
type fakeClient struct {
	listOutputs    []*awssm.ListSecretsOutput
	listInputs     []*awssm.ListSecretsInput
	listErr        error
	listCalls      int
	describeOutput *awssm.DescribeSecretOutput
	describeErr    error
	getOutput      *awssm.GetSecretValueOutput
	getErr         error
	createInput    *awssm.CreateSecretInput
	createErr      error
	putInput       *awssm.PutSecretValueInput
	putErr         error
	restoreInput   *awssm.RestoreSecretInput
	restoreErr     error
	deleteInput    *awssm.DeleteSecretInput
	deleteErr      error
	getCalls       int
}

// ListSecrets returns configured paginated scope candidates.
func (client *fakeClient) ListSecrets(
	_ context.Context,
	input *awssm.ListSecretsInput,
	_ ...func(*awssm.Options),
) (*awssm.ListSecretsOutput, error) {
	client.listInputs = append(client.listInputs, input)
	if client.listErr != nil {
		return nil, client.listErr
	}
	if client.listCalls < len(client.listOutputs) {
		output := client.listOutputs[client.listCalls]
		client.listCalls++
		return output, nil
	}

	return &awssm.ListSecretsOutput{}, nil
}

// TestDiscoverPaginatesPlannedDeletionCandidates proves list filters remain an optimization only.
func TestDiscoverPaginatesPlannedDeletionCandidates(t *testing.T) {
	t.Parallel()

	deletedAt := time.Now()
	client := &fakeClient{listOutputs: []*awssm.ListSecretsOutput{
		{
			SecretList: []types.SecretListEntry{{
				Name: aws.String("/acme/payments/removed"), DeletedDate: &deletedAt,
				Tags: []types.Tag{
					{Key: aws.String(domain.ManagedByTagKey), Value: aws.String(domain.ManagedByTagValue)},
					{Key: aws.String(domain.ScopeTagKey), Value: aws.String("scope-sentinel")},
				},
			}},
			NextToken: aws.String("page-2"),
		},
		{SecretList: []types.SecretListEntry{{Name: aws.String("/acme/payments/other")}}},
	}}
	adapter, err := New(client, time.Second)
	require.NoError(t, err)

	discovered, err := adapter.Discover(context.Background())
	require.NoError(t, err)
	require.Len(t, discovered, 2)
	assert.Equal(t, "/acme/payments/removed", discovered[0].Name.Value())
	assert.True(t, discovered[0].Evidence.ScheduledForDeletion)
	assert.Equal(t, "scope-sentinel", discovered[0].Evidence.ReservedTags[domain.ScopeTagKey])
	require.Len(t, client.listInputs, 2)
	assert.True(t, aws.ToBool(client.listInputs[0].IncludePlannedDeletion))
	assert.Equal(t, "page-2", aws.ToString(client.listInputs[1].NextToken))
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
	client.getCalls++
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

// RestoreSecret records one lifecycle restore request.
func (client *fakeClient) RestoreSecret(
	_ context.Context,
	input *awssm.RestoreSecretInput,
	_ ...func(*awssm.Options),
) (*awssm.RestoreSecretOutput, error) {
	client.restoreInput = input

	return &awssm.RestoreSecretOutput{}, client.restoreErr
}

// DeleteSecret records one recovery-window deletion request.
func (client *fakeClient) DeleteSecret(
	_ context.Context,
	input *awssm.DeleteSecretInput,
	_ ...func(*awssm.Options),
) (*awssm.DeleteSecretOutput, error) {
	client.deleteInput = input

	return &awssm.DeleteSecretOutput{}, client.deleteErr
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

	evidence, err := adapter.Observe(context.Background(), desired, scope)
	require.NoError(t, err)
	assert.True(t, evidence.Exists)
	assert.True(t, evidence.StagingValid)
	assert.Equal(t, "version-1", evidence.CurrentVersion)
	assert.Len(t, evidence.ReservedTags, 3)
	assert.Equal(t, domain.ObservedOwnedActiveString, domain.ClassifyDirect(desired, scope, evidence).Kind())
}

// TestObserveDoesNotReadPayloadBeforeMetadataOwnership proves conflicts remain metadata-only.
func TestObserveDoesNotReadPayloadBeforeMetadataOwnership(t *testing.T) {
	t.Parallel()

	desired, scope := adapterDesired(t)
	tests := []struct {
		name   string
		mutate func(*awssm.DescribeSecretOutput)
		want   domain.ObservedKind
	}{
		{
			name: "foreign ownership",
			mutate: func(output *awssm.DescribeSecretOutput) {
				output.Tags = nil
			},
			want: domain.ObservedForeign,
		},
		{
			name: "service owned",
			mutate: func(output *awssm.DescribeSecretOutput) {
				output.OwningService = aws.String("rds")
			},
			want: domain.ObservedConflict,
		},
		{
			name: "rotation enabled",
			mutate: func(output *awssm.DescribeSecretOutput) {
				output.RotationEnabled = aws.Bool(true)
			},
			want: domain.ObservedConflict,
		},
		{
			name: "replicated",
			mutate: func(output *awssm.DescribeSecretOutput) {
				output.ReplicationStatus = []types.ReplicationStatusType{{Region: aws.String("us-west-2")}}
			},
			want: domain.ObservedConflict,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			description := ownedDescription(desired, scope)
			test.mutate(description)
			client := &fakeClient{describeOutput: description, getErr: errors.New("payload access denied sentinel")}
			adapter, err := New(client, time.Second)
			require.NoError(t, err)

			evidence, err := adapter.Observe(context.Background(), desired, scope)
			require.NoError(t, err)
			assert.Equal(t, test.want, domain.ClassifyDirect(desired, scope, evidence).Kind())
			assert.Zero(t, client.getCalls)
		})
	}
}

// TestObserveRejectsCurrentVersionWithoutPayload proves inconsistent value evidence fails closed.
func TestObserveRejectsCurrentVersionWithoutPayload(t *testing.T) {
	t.Parallel()

	desired, scope := adapterDesired(t)
	client := &fakeClient{
		describeOutput: ownedDescription(desired, scope),
		getOutput: &awssm.GetSecretValueOutput{
			VersionId: aws.String("version-1"), VersionStages: []string{awsCurrent},
		},
	}
	adapter, err := New(client, time.Second)
	require.NoError(t, err)

	evidence, err := adapter.Observe(context.Background(), desired, scope)
	require.NoError(t, err)
	assert.Equal(t, domain.ObservedInvalid, domain.ClassifyDirect(desired, scope, evidence).Kind())
	assert.Equal(t, 1, client.getCalls)
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

// TestRestoreAndScheduleDeletionUseSafeLifecycleRequests proves force deletion is impossible.
func TestRestoreAndScheduleDeletionUseSafeLifecycleRequests(t *testing.T) {
	t.Parallel()

	desired, _ := adapterDesired(t)
	client := &fakeClient{}
	adapter, err := New(client, time.Second)
	require.NoError(t, err)

	require.NoError(t, adapter.Restore(context.Background(), desired.Name()))
	require.NotNil(t, client.restoreInput)
	assert.Equal(t, desired.Name().Value(), aws.ToString(client.restoreInput.SecretId))

	require.NoError(t, adapter.ScheduleDeletion(context.Background(), desired.Name(), 14))
	require.NotNil(t, client.deleteInput)
	assert.Equal(t, desired.Name().Value(), aws.ToString(client.deleteInput.SecretId))
	assert.EqualValues(t, 14, aws.ToInt64(client.deleteInput.RecoveryWindowInDays))
	assert.Nil(t, client.deleteInput.ForceDeleteWithoutRecovery)
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

// TestMutationConnectionLossIsTypedAmbiguous proves non-timeout transport loss is re-observed.
func TestMutationConnectionLossIsTypedAmbiguous(t *testing.T) {
	t.Parallel()

	connectionLoss := &net.OpError{Op: "write", Net: "tcp", Err: errors.New("connection reset sentinel")}
	safe := classifyError("update", connectionLoss, true)
	assert.True(t, safe.Ambiguous())
	assert.False(t, safe.Canceled())
	assert.NotContains(t, safe.Error(), "connection reset sentinel")
}

// TestSDKRetriesTransientCreateWithTheSameToken proves bounded request-level retry behavior.
func TestSDKRetriesTransientCreateWithTheSameToken(t *testing.T) {
	t.Parallel()

	const expectedAttempts = 3
	var attempts atomic.Int32
	var lock sync.Mutex
	tokens := make([]string, 0, expectedAttempts)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var payload struct {
			// ClientRequestToken is the logical-write idempotency token.
			ClientRequestToken string `json:"ClientRequestToken"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		lock.Lock()
		tokens = append(tokens, payload.ClientRequestToken)
		lock.Unlock()
		if attempts.Add(1) < expectedAttempts {
			response.Header().Set("X-Amzn-Errortype", "InternalServiceError")
			response.WriteHeader(http.StatusInternalServerError)
			_, _ = response.Write([]byte(`{"__type":"InternalServiceError","message":"transient sentinel"}`))
			return
		}
		response.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = response.Write([]byte(`{}`))
	}))
	defer server.Close()
	awsConfiguration := aws.Config{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(server.URL),
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
		HTTPClient:   server.Client(),
		Retryer: func() aws.Retryer {
			return retry.NewStandard(func(options *retry.StandardOptions) {
				options.MaxAttempts = expectedAttempts
				options.Backoff = retry.BackoffDelayerFunc(func(_ int, _ error) (time.Duration, error) {
					return 0, nil
				})
			})
		},
	}
	adapter, err := New(awssm.NewFromConfig(awsConfiguration), time.Second)
	require.NoError(t, err)
	desired, scope := adapterDesired(t)
	expectedToken := "0123456789abcdef0123456789abcdef"

	require.NoError(t, adapter.Create(context.Background(), desired, scope, expectedToken))
	assert.EqualValues(t, expectedAttempts, attempts.Load())
	lock.Lock()
	defer lock.Unlock()
	assert.Equal(t, []string{expectedToken, expectedToken, expectedToken}, tokens)
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

// ownedDescription returns metadata that requires one current payload comparison.
func ownedDescription(desired domain.DesiredSecret, scope domain.ScopeIdentity) *awssm.DescribeSecretOutput {
	return &awssm.DescribeSecretOutput{
		Tags: []types.Tag{
			{Key: aws.String(domain.ManagedByTagKey), Value: aws.String(domain.ManagedByTagValue)},
			{Key: aws.String(domain.ScopeTagKey), Value: aws.String(scope.Value())},
			{Key: aws.String(domain.SourceTagKey), Value: aws.String(desired.Source().Value())},
		},
		VersionIdsToStages: map[string][]string{"version-1": {awsCurrent}},
	}
}
