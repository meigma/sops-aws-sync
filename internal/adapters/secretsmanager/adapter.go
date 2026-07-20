package secretsmanager

import (
	"context"
	"errors"
	"net"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awssm "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/aws/smithy-go"

	"github.com/meigma/sops-aws-sync/internal/domain"
)

const (
	awsCurrent         = "AWSCURRENT"
	awsPending         = "AWSPENDING"
	reservedTagCount   = 3
	loadOptionCapacity = 3
)

// loadError retains cancellation and setup classification behind a safe message.
type loadError struct {
	configuration bool
	cause         error
}

// Error returns a stable message that does not disclose credential-provider details.
func (failure *loadError) Error() string {
	if failure.configuration {
		return "AWS configuration is invalid"
	}

	return "AWS credential loading failed"
}

// Unwrap retains cancellation and deadline evidence for the composition root.
func (failure *loadError) Unwrap() error {
	return failure.cause
}

// ConfigurationFailure distinguishes operator configuration from provider availability.
func (failure *loadError) ConfigurationFailure() bool {
	return failure.configuration
}

// Client is the consumed AWS Secrets Manager SDK surface.
type Client interface {
	DescribeSecret(
		ctx context.Context,
		input *awssm.DescribeSecretInput,
		optFns ...func(*awssm.Options),
	) (*awssm.DescribeSecretOutput, error)
	GetSecretValue(
		ctx context.Context,
		input *awssm.GetSecretValueInput,
		optFns ...func(*awssm.Options),
	) (*awssm.GetSecretValueOutput, error)
	CreateSecret(
		ctx context.Context,
		input *awssm.CreateSecretInput,
		optFns ...func(*awssm.Options),
	) (*awssm.CreateSecretOutput, error)
	PutSecretValue(
		ctx context.Context,
		input *awssm.PutSecretValueInput,
		optFns ...func(*awssm.Options),
	) (*awssm.PutSecretValueOutput, error)
}

// Adapter performs direct desired-name observation and Phase 2 writes.
type Adapter struct {
	client           Client
	operationTimeout time.Duration
}

// LoadOptions configures standard-chain AWS loading and bounded SDK behavior.
type LoadOptions struct {
	// Region optionally overrides the standard Region chain.
	Region string
	// Profile optionally selects a shared configuration profile.
	Profile string
	// MaxAttempts optionally overrides the standard retry attempt bound.
	MaxAttempts int
	// MaxBackoff optionally caps retry backoff.
	MaxBackoff time.Duration
	// OperationTimeout bounds each AWS call.
	OperationTimeout time.Duration
}

// New constructs an adapter around an SDK-compatible client.
func New(client Client, operationTimeout time.Duration) (*Adapter, error) {
	if client == nil {
		return nil, errors.New("secrets manager client is required")
	}
	if operationTimeout <= 0 {
		return nil, errors.New("operation timeout must be positive")
	}

	return &Adapter{client: client, operationTimeout: operationTimeout}, nil
}

// Load constructs the standard-chain AWS adapter after validating credentials and Region.
func Load(ctx context.Context, configuration LoadOptions) (*Adapter, error) {
	loadOptions := make([]func(*awsconfig.LoadOptions) error, 0, loadOptionCapacity)
	if configuration.Region != "" {
		loadOptions = append(loadOptions, awsconfig.WithRegion(configuration.Region))
	}
	if configuration.Profile != "" {
		loadOptions = append(loadOptions, awsconfig.WithSharedConfigProfile(configuration.Profile))
	}
	if configuration.MaxAttempts > 0 || configuration.MaxBackoff > 0 {
		loadOptions = append(loadOptions, awsconfig.WithRetryer(func() aws.Retryer {
			return retry.NewStandard(func(retryOptions *retry.StandardOptions) {
				if configuration.MaxAttempts > 0 {
					retryOptions.MaxAttempts = configuration.MaxAttempts
				}
				if configuration.MaxBackoff > 0 {
					retryOptions.MaxBackoff = configuration.MaxBackoff
				}
			})
		}))
	}
	awsConfiguration, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, &loadError{configuration: true, cause: err}
	}
	if awsConfiguration.Region == "" {
		return nil, &loadError{configuration: true, cause: errors.New("AWS Region is not configured")}
	}
	if _, retrieveErr := awsConfiguration.Credentials.Retrieve(ctx); retrieveErr != nil {
		return nil, &loadError{cause: retrieveErr}
	}

	adapter, err := New(awssm.NewFromConfig(awsConfiguration), configuration.OperationTimeout)
	if err != nil {
		return nil, &loadError{configuration: true, cause: err}
	}

	return adapter, nil
}

// Observe classifies direct metadata before loading an owned current payload.
func (adapter *Adapter) Observe(
	ctx context.Context,
	desired domain.DesiredSecret,
	scope domain.ScopeIdentity,
) (domain.ObservedEvidence, error) {
	name := desired.Name()
	callContext, cancel := context.WithTimeout(ctx, adapter.operationTimeout)
	defer cancel()
	description, err := adapter.client.DescribeSecret(
		callContext,
		&awssm.DescribeSecretInput{SecretId: aws.String(name.Value())},
	)
	if err != nil {
		var notFound *types.ResourceNotFoundException
		if errors.As(err, &notFound) {
			return domain.ObservedEvidence{}, nil
		}
		return domain.ObservedEvidence{}, classifyError("describe", err, false)
	}
	evidence := evidenceFromDescription(description)
	if !domain.CurrentPayloadRequired(desired, scope, evidence) {
		return evidence, nil
	}
	callContext, cancel = context.WithTimeout(ctx, adapter.operationTimeout)
	defer cancel()
	current, err := adapter.client.GetSecretValue(callContext, &awssm.GetSecretValueInput{
		SecretId:     aws.String(name.Value()),
		VersionId:    aws.String(evidence.CurrentVersion),
		VersionStage: aws.String(awsCurrent),
	})
	if err != nil {
		return domain.ObservedEvidence{}, classifyError("get-current", err, false)
	}
	if aws.ToString(current.VersionId) != evidence.CurrentVersion ||
		!slices.Contains(current.VersionStages, awsCurrent) {
		evidence.StagingValid = false
		return evidence, nil
	}
	evidence.SecretString = current.SecretString
	evidence.SecretBinary = current.SecretBinary != nil

	return evidence, nil
}

// evidenceFromDescription normalizes SDK metadata without exposing SDK types to the domain.
func evidenceFromDescription(description *awssm.DescribeSecretOutput) domain.ObservedEvidence {
	reservedTags := make(map[string]string, reservedTagCount)
	for _, tag := range description.Tags {
		key := aws.ToString(tag.Key)
		if key == domain.ManagedByTagKey || key == domain.ScopeTagKey || key == domain.SourceTagKey {
			reservedTags[key] = aws.ToString(tag.Value)
		}
	}
	currentVersions := make([]string, 0, 1)
	rotationInProgress := false
	for version, stages := range description.VersionIdsToStages {
		if slices.Contains(stages, awsCurrent) {
			currentVersions = append(currentVersions, version)
		}
		if slices.Contains(stages, awsPending) {
			rotationInProgress = true
		}
	}
	owner := aws.ToString(description.OwningService)
	if owner == "" {
		owner = aws.ToString(description.Type)
	}
	evidence := domain.ObservedEvidence{
		Exists:               true,
		ScheduledForDeletion: description.DeletedDate != nil,
		ReservedTags:         reservedTags,
		OwningService:        owner,
		RotationEnabled:      aws.ToBool(description.RotationEnabled),
		RotationInProgress:   rotationInProgress,
		HasReplicas:          len(description.ReplicationStatus) > 0,
		StagingValid:         len(currentVersions) <= 1,
	}
	if len(currentVersions) == 1 {
		evidence.CurrentVersion = currentVersions[0]
	}

	return evidence
}

// Create writes one canonical value with the exact reserved ownership tags.
func (adapter *Adapter) Create(
	ctx context.Context,
	desired domain.DesiredSecret,
	scope domain.ScopeIdentity,
	token string,
) error {
	callContext, cancel := context.WithTimeout(ctx, adapter.operationTimeout)
	defer cancel()
	canonical := string(desired.Value().CopyCanonicalJSON())
	_, err := adapter.client.CreateSecret(callContext, &awssm.CreateSecretInput{
		ClientRequestToken: aws.String(token),
		Name:               aws.String(desired.Name().Value()),
		SecretString:       aws.String(canonical),
		Tags: []types.Tag{
			{Key: aws.String(domain.ManagedByTagKey), Value: aws.String(domain.ManagedByTagValue)},
			{Key: aws.String(domain.ScopeTagKey), Value: aws.String(scope.Value())},
			{Key: aws.String(domain.SourceTagKey), Value: aws.String(desired.Source().Value())},
		},
	})
	if err != nil {
		return classifyError("create", err, true)
	}

	return nil
}

// Update writes one new AWSCURRENT canonical string version.
func (adapter *Adapter) Update(ctx context.Context, desired domain.DesiredSecret, token string) error {
	callContext, cancel := context.WithTimeout(ctx, adapter.operationTimeout)
	defer cancel()
	canonical := string(desired.Value().CopyCanonicalJSON())
	_, err := adapter.client.PutSecretValue(callContext, &awssm.PutSecretValueInput{
		ClientRequestToken: aws.String(token),
		SecretId:           aws.String(desired.Name().Value()),
		SecretString:       aws.String(canonical),
		VersionStages:      []string{awsCurrent},
	})
	if err != nil {
		return classifyError("update", err, true)
	}

	return nil
}

// SafeError is a normalized AWS failure that never contains raw service text.
type SafeError struct {
	operation string
	code      string
	fault     string
	requestID string
	ambiguous bool
	canceled  bool
}

// Error returns a fixed non-sensitive failure summary.
func (failure *SafeError) Error() string {
	return "AWS " + failure.operation + " failed"
}

// Code returns the modeled safe AWS error code when present.
func (failure *SafeError) Code() string {
	return failure.code
}

// Operation returns the stable AWS operation kind.
func (failure *SafeError) Operation() string {
	return failure.operation
}

// Fault returns the modeled client or server fault class.
func (failure *SafeError) Fault() string {
	return failure.fault
}

// RequestID returns the AWS request identifier when present.
func (failure *SafeError) RequestID() string {
	return failure.requestID
}

// Ambiguous reports whether a mutation may have reached the service.
func (failure *SafeError) Ambiguous() bool {
	return failure.ambiguous
}

// Canceled reports whether the run or operation context ended.
func (failure *SafeError) Canceled() bool {
	return failure.canceled
}

// classifyError reduces SDK and transport failures to safe typed evidence.
func classifyError(operation string, err error, mutation bool) *SafeError {
	failure := &SafeError{operation: operation}
	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		failure.code = apiError.ErrorCode()
		failure.fault = apiError.ErrorFault().String()
	}
	var requestError interface{ ServiceRequestID() string }
	if errors.As(err, &requestError) {
		failure.requestID = requestError.ServiceRequestID()
	}
	var networkError net.Error
	networkFailure := errors.As(err, &networkError)
	timedOut := errors.Is(err, context.DeadlineExceeded) || networkFailure && networkError.Timeout()
	failure.canceled = errors.Is(err, context.Canceled)
	failure.ambiguous = mutation && (timedOut || failure.canceled || networkFailure)

	return failure
}
