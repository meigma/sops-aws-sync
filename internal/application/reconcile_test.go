package application_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/sops-aws-sync/internal/application"
	"github.com/meigma/sops-aws-sync/internal/domain"
)

// fakeSource returns one fixed committed document snapshot.
type fakeSource struct {
	snapshot application.SourceSnapshot
}

// Load returns the configured exact source snapshot.
func (source *fakeSource) Load(
	_ context.Context,
	_, _ string,
) (application.SourceSnapshot, error) {
	return source.snapshot, nil
}

// fakeDecrypter returns one fixed canonical domain value.
type fakeDecrypter struct {
	value domain.SecretValue
	err   error
}

// DecryptJSON returns the configured canonical value.
func (decrypter *fakeDecrypter) DecryptJSON(
	_ context.Context,
	_ []byte,
) (domain.SecretValue, error) {
	return decrypter.value, decrypter.err
}

// sequenceTokens records deterministic fresh token allocation.
type sequenceTokens struct {
	next int
}

// NewToken returns a distinct valid token for each logical write.
func (tokens *sequenceTokens) NewToken() (string, error) {
	tokens.next++

	return fmt.Sprintf("%032d", tokens.next), nil
}

// ambiguousMutationError models a lost mutation response.
type ambiguousMutationError struct{}

// Error returns a safe fixed message.
func (ambiguousMutationError) Error() string {
	return "ambiguous mutation"
}

// Ambiguous reports that the service may have applied the write.
func (ambiguousMutationError) Ambiguous() bool {
	return true
}

// Canceled reports that the parent run remains active.
func (ambiguousMutationError) Canceled() bool {
	return false
}

// memorySecrets models direct desired-name AWS evidence and mutation calls.
type memorySecrets struct {
	evidence        domain.ObservedEvidence
	scope           domain.ScopeIdentity
	createCalls     int
	updateCalls     int
	tokens          []string
	ambiguousApply  bool
	ambiguousBefore bool
	ambiguousUpdate bool
	observeErr      error
	observeCalls    int
	observeDelay    time.Duration
	observations    []domain.ObservedEvidence
	observationErrs []error
}

// orderingSecrets introduces owned drift before the first create and records rebuilt phase order.
type orderingSecrets struct {
	current      map[string]domain.ObservedEvidence
	observeCalls map[string]int
	operations   []string
	driftName    string
}

// Observe introduces one create-to-update transition and otherwise returns stored evidence.
func (secrets *orderingSecrets) Observe(
	_ context.Context,
	desired domain.DesiredSecret,
	scope domain.ScopeIdentity,
) (domain.ObservedEvidence, error) {
	name := desired.Name().Value()
	call := secrets.observeCalls[name]
	secrets.observeCalls[name]++
	if name == secrets.driftName && call == 1 {
		secrets.current[name] = ownedApplicationEvidenceWithValue(desired, scope, `{"password":"old"}`)
	}

	return secrets.current[name], nil
}

// Create records a phase-ordered create and converges the stored evidence.
func (secrets *orderingSecrets) Create(
	_ context.Context,
	desired domain.DesiredSecret,
	scope domain.ScopeIdentity,
	_ string,
) error {
	name := desired.Name().Value()
	secrets.operations = append(secrets.operations, "create:"+name)
	secrets.current[name] = ownedApplicationEvidence(desired, scope)

	return nil
}

// Update records a phase-ordered update and converges the stored evidence.
func (secrets *orderingSecrets) Update(
	_ context.Context,
	desired domain.DesiredSecret,
	_ string,
) error {
	name := desired.Name().Value()
	secrets.operations = append(secrets.operations, "update:"+name)
	current := secrets.current[name]
	value := string(desired.Value().CopyCanonicalJSON())
	current.SecretString = &value
	secrets.current[name] = current

	return nil
}

// Observe returns the current direct desired-name evidence.
func (secrets *memorySecrets) Observe(
	_ context.Context,
	_ domain.DesiredSecret,
	_ domain.ScopeIdentity,
) (domain.ObservedEvidence, error) {
	if secrets.observeDelay > 0 {
		time.Sleep(secrets.observeDelay)
	}
	index := secrets.observeCalls
	secrets.observeCalls++
	if index < len(secrets.observationErrs) && secrets.observationErrs[index] != nil {
		return domain.ObservedEvidence{}, secrets.observationErrs[index]
	}
	if index < len(secrets.observations) {
		secrets.evidence = secrets.observations[index]
	}
	return secrets.evidence, secrets.observeErr
}

// Create records a create and optionally simulates an ambiguous response.
func (secrets *memorySecrets) Create(
	_ context.Context,
	desired domain.DesiredSecret,
	scope domain.ScopeIdentity,
	token string,
) error {
	secrets.createCalls++
	secrets.tokens = append(secrets.tokens, token)
	if secrets.ambiguousBefore && secrets.createCalls == 1 {
		return ambiguousMutationError{}
	}
	secrets.scope = scope
	secrets.evidence = ownedApplicationEvidence(desired, scope)
	if secrets.ambiguousApply && secrets.createCalls == 1 {
		return ambiguousMutationError{}
	}

	return nil
}

// Update records an update and changes the current direct payload.
func (secrets *memorySecrets) Update(
	_ context.Context,
	desired domain.DesiredSecret,
	token string,
) error {
	secrets.updateCalls++
	secrets.tokens = append(secrets.tokens, token)
	secrets.evidence = ownedApplicationEvidence(desired, secrets.scope)
	if secrets.ambiguousUpdate && secrets.updateCalls == 1 {
		return ambiguousMutationError{}
	}

	return nil
}

// appTestContext groups one service and its observable fake collaborators.
type appTestContext struct {
	service *application.Service
	secrets *memorySecrets
	tokens  *sequenceTokens
	desired domain.DesiredSecret
	input   application.ReconcileInput
	logs    *bytes.Buffer
}

// TestSyncCreateUpdateAndNoOp proves the supported Phase 2 vertical slice.
func TestSyncCreateUpdateAndNoOp(t *testing.T) {
	t.Parallel()

	testContext := newAppTestContext(t)
	report, err := testContext.service.Sync(context.Background(), testContext.input)
	require.NoError(t, err)
	assert.Equal(t, 1, testContext.secrets.createCalls)
	assert.Equal(t, "converged", report.Status)
	assert.Equal(t, "converged", report.Verification)

	old := `{"password":"old"}`
	testContext.secrets.evidence = ownedApplicationEvidenceWithValue(
		testContext.desired,
		testContext.secrets.scope,
		old,
	)
	report, err = testContext.service.Sync(context.Background(), testContext.input)
	require.NoError(t, err)
	assert.Equal(t, 1, testContext.secrets.updateCalls)
	assert.Equal(t, 1, report.Counts.Update)

	report, err = testContext.service.Sync(context.Background(), testContext.input)
	require.NoError(t, err)
	assert.Equal(t, 1, testContext.secrets.updateCalls, "identical content must not create another version")
	assert.Equal(t, 1, report.Counts.Unchanged)
}

// TestSyncUsesFreshTokenToRepairRepeatedExternalDrift proves tokens are per logical write.
func TestSyncUsesFreshTokenToRepairRepeatedExternalDrift(t *testing.T) {
	t.Parallel()

	testContext := newAppTestContext(t)
	testContext.secrets.scope = applicationScope(t)
	old := `{"password":"old"}`
	testContext.secrets.evidence = ownedApplicationEvidenceWithValue(
		testContext.desired,
		testContext.secrets.scope,
		old,
	)
	_, err := testContext.service.Sync(context.Background(), testContext.input)
	require.NoError(t, err)
	testContext.secrets.evidence = ownedApplicationEvidenceWithValue(
		testContext.desired,
		testContext.secrets.scope,
		old,
	)
	_, err = testContext.service.Sync(context.Background(), testContext.input)
	require.NoError(t, err)

	require.Len(t, testContext.secrets.tokens, 2)
	assert.NotEqual(t, testContext.secrets.tokens[0], testContext.secrets.tokens[1])
}

// TestSyncResolvesAmbiguousCreateWithoutBlindRetry proves successful lost responses re-observe first.
func TestSyncResolvesAmbiguousCreateWithoutBlindRetry(t *testing.T) {
	t.Parallel()

	testContext := newAppTestContext(t)
	testContext.secrets.ambiguousApply = true
	report, err := testContext.service.Sync(context.Background(), testContext.input)
	require.NoError(t, err)
	assert.Equal(t, 1, testContext.secrets.createCalls)
	assert.Equal(t, "converged", report.Status)
}

// TestSyncRetriesAmbiguousLogicalWriteWithSameToken proves the bounded retry transition.
func TestSyncRetriesAmbiguousLogicalWriteWithSameToken(t *testing.T) {
	t.Parallel()

	testContext := newAppTestContext(t)
	testContext.secrets.ambiguousBefore = true
	_, err := testContext.service.Sync(context.Background(), testContext.input)
	require.NoError(t, err)
	require.Len(t, testContext.secrets.tokens, 2)
	assert.Equal(t, testContext.secrets.tokens[0], testContext.secrets.tokens[1])
}

// TestSyncResolvesAmbiguousUpdateWithoutBlindRetry proves lost update responses re-observe first.
func TestSyncResolvesAmbiguousUpdateWithoutBlindRetry(t *testing.T) {
	t.Parallel()

	testContext := newAppTestContext(t)
	testContext.secrets.scope = applicationScope(t)
	testContext.secrets.evidence = ownedApplicationEvidenceWithValue(
		testContext.desired,
		testContext.secrets.scope,
		`{"password":"old"}`,
	)
	testContext.secrets.ambiguousUpdate = true
	report, err := testContext.service.Sync(context.Background(), testContext.input)
	require.NoError(t, err)
	assert.Equal(t, 1, testContext.secrets.updateCalls)
	assert.Equal(t, "converged", report.Status)
}

// TestSyncRebuildsWholePlanAfterHarmlessDrift proves phase order and counts come from the replacement plan.
func TestSyncRebuildsWholePlanAfterHarmlessDrift(t *testing.T) {
	t.Parallel()

	value, err := domain.NewSecretValue([]byte(`{"password":"desired"}`))
	require.NoError(t, err)
	revision, err := domain.NewRevision("0123456789abcdef0123456789abcdef01234567")
	require.NoError(t, err)
	source := &fakeSource{snapshot: application.SourceSnapshot{
		Revision: revision,
		Documents: []application.EncryptedDocument{
			{Path: "secrets/a.sops.json", Data: []byte("encrypted-a")},
			{Path: "secrets/b.sops.json", Data: []byte("encrypted-b")},
		},
	}}
	snapshot, err := application.BuildDesiredSnapshot(context.Background(), source, &fakeDecrypter{value: value},
		application.DesiredInput{Revision: "HEAD", SourceRoot: "secrets", SecretPrefix: "/acme/payments"})
	require.NoError(t, err)
	secrets := &orderingSecrets{
		current: map[string]domain.ObservedEvidence{}, observeCalls: map[string]int{},
		driftName: "/acme/payments/a",
	}
	service, err := application.NewService(
		snapshot, secrets, &sequenceTokens{}, slog.Default(), "test",
	)
	require.NoError(t, err)
	report, err := service.Sync(context.Background(), application.ReconcileInput{
		RepositoryID: "meigma/example", SourceRoot: "secrets", SecretPrefix: "/acme/payments",
		VerificationTimeout: time.Second,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"create:/acme/payments/b",
		"update:/acme/payments/a",
	}, secrets.operations)
	assert.Equal(t, 1, report.Counts.Create)
	assert.Equal(t, 1, report.Counts.Update)
	assert.Equal(t, "converged", report.Status)
}

// TestSyncBoundsRepeatedPreconditionReplans proves concurrent churn cannot loop indefinitely.
func TestSyncBoundsRepeatedPreconditionReplans(t *testing.T) {
	t.Parallel()

	testContext := newAppTestContext(t)
	old := ownedApplicationEvidenceWithValue(
		testContext.desired,
		testContext.secrets.scope,
		`{"password":"old"}`,
	)
	testContext.secrets.observations = []domain.ObservedEvidence{{}, old, {}, old}
	report, err := testContext.service.Sync(context.Background(), testContext.input)
	var outcome *application.OutcomeError
	require.ErrorAs(t, err, &outcome)
	assert.Equal(t, application.OutcomeVerification, outcome.Kind())
	assert.Equal(t, "verification-failed", report.Status)
	assert.Equal(t, "failed", report.Verification)
	assert.Zero(t, testContext.secrets.createCalls+testContext.secrets.updateCalls)
}

// TestSyncApplyOutcomeMatchesReport proves machine status agrees with the stable exit class.
func TestSyncApplyOutcomeMatchesReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		observations     []domain.ObservedEvidence
		observationErrs  []error
		wantKind         application.OutcomeKind
		wantStatus       string
		wantVerification string
	}{
		{
			name: "ownership conflict",
			observations: []domain.ObservedEvidence{
				{},
				{Exists: true},
			},
			wantKind: application.OutcomeConflict, wantStatus: "conflict", wantVerification: "not-run",
		},
		{
			name:            "interruption",
			observationErrs: []error{nil, context.Canceled},
			wantKind:        application.OutcomeInterrupted, wantStatus: "interrupted", wantVerification: "not-run",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			testContext := newAppTestContext(t)
			testContext.secrets.observations = test.observations
			testContext.secrets.observationErrs = test.observationErrs
			report, err := testContext.service.Sync(context.Background(), testContext.input)
			var outcome *application.OutcomeError
			require.ErrorAs(t, err, &outcome)
			assert.Equal(t, test.wantKind, outcome.Kind())
			assert.Equal(t, test.wantStatus, report.Status)
			assert.Equal(t, test.wantVerification, report.Verification)
			assert.Zero(t, testContext.secrets.createCalls+testContext.secrets.updateCalls)
		})
	}
}

// TestSyncCancellationStopsBeforeMutation proves parent interruption maps to exit 130 behavior.
func TestSyncCancellationStopsBeforeMutation(t *testing.T) {
	t.Parallel()

	testContext := newAppTestContext(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := testContext.service.Sync(ctx, testContext.input)
	var outcome *application.OutcomeError
	require.ErrorAs(t, err, &outcome)
	assert.Equal(t, application.OutcomeInterrupted, outcome.Kind())
	assert.Zero(t, testContext.secrets.createCalls+testContext.secrets.updateCalls)
}

// TestSyncConflictPerformsNoMutation proves same-name ownership conflicts fail closed.
func TestSyncConflictPerformsNoMutation(t *testing.T) {
	t.Parallel()

	testContext := newAppTestContext(t)
	testContext.secrets.observeDelay = 5 * time.Millisecond
	testContext.secrets.evidence = domain.ObservedEvidence{Exists: true}
	report, err := testContext.service.Sync(context.Background(), testContext.input)
	var outcome *application.OutcomeError
	require.ErrorAs(t, err, &outcome)
	assert.Equal(t, application.OutcomeConflict, outcome.Kind())
	assert.Equal(t, "conflict", report.Status)
	assert.Equal(t, "not-run", report.Verification)
	assert.GreaterOrEqual(t, report.DurationMilliseconds, int64(5))
	assert.Zero(t, testContext.secrets.createCalls+testContext.secrets.updateCalls)
}

// TestPlanObservationFailureUsesOperationalExitClass proves service failures are not invalid input.
func TestPlanObservationFailureUsesOperationalExitClass(t *testing.T) {
	t.Parallel()

	testContext := newAppTestContext(t)
	testContext.secrets.observeErr = errors.New("transport sentinel must not escape")
	report, err := testContext.service.Plan(context.Background(), testContext.input)
	var outcome *application.OutcomeError
	require.ErrorAs(t, err, &outcome)
	assert.Equal(t, application.OutcomeApplyFailed, outcome.Kind())
	assert.Equal(t, "observation-failed", report.Status)
	assert.NotContains(t, testContext.logs.String(), "transport sentinel")
}

// TestPlanReportAndLogsDoNotDiscloseSentinels proves default output remains non-sensitive.
func TestPlanReportAndLogsDoNotDiscloseSentinels(t *testing.T) {
	t.Parallel()

	testContext := newAppTestContext(t)
	report, err := testContext.service.Plan(context.Background(), testContext.input)
	require.NoError(t, err)
	encoded, err := json.Marshal(report)
	require.NoError(t, err)
	output := string(encoded) + testContext.logs.String()
	assert.NotContains(t, output, "phase2-plaintext-sentinel")
	assert.NotContains(t, output, "/acme/payments/database")
	assert.NotContains(t, output, "secrets/database.sops.json")
}

// newAppTestContext builds one application service with deterministic fakes.
func newAppTestContext(t *testing.T) *appTestContext {
	t.Helper()
	name, err := domain.NewSecretName("/acme/payments/database")
	require.NoError(t, err)
	sourceIdentity, err := domain.NewSourceIdentity("secrets/database.sops.json")
	require.NoError(t, err)
	value, err := domain.NewSecretValue([]byte(`{"password":"phase2-plaintext-sentinel"}`))
	require.NoError(t, err)
	revision, err := domain.NewRevision("0123456789abcdef0123456789abcdef01234567")
	require.NoError(t, err)
	desired := domain.NewDesiredSecret(name, sourceIdentity, value, revision)
	source := &fakeSource{snapshot: application.SourceSnapshot{
		Revision:  revision,
		Documents: []application.EncryptedDocument{{Path: "secrets/database.sops.json", Data: []byte("encrypted")}},
	}}
	secrets := &memorySecrets{scope: applicationScope(t)}
	tokens := &sequenceTokens{}
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	decrypter := &fakeDecrypter{value: value}
	desiredSnapshot, err := application.BuildDesiredSnapshot(context.Background(), source, decrypter,
		application.DesiredInput{Revision: "HEAD", SourceRoot: "secrets", SecretPrefix: "/acme/payments"})
	require.NoError(t, err)
	service, err := application.NewService(desiredSnapshot, secrets, tokens, logger, "test")
	require.NoError(t, err)

	return &appTestContext{
		service: service,
		secrets: secrets,
		tokens:  tokens,
		desired: desired,
		input: application.ReconcileInput{
			RepositoryID: "meigma/example", SourceRoot: "secrets",
			SecretPrefix: "/acme/payments", VerificationTimeout: time.Second,
		},
		logs: logs,
	}
}

// applicationScope returns the exact test ownership scope.
func applicationScope(t *testing.T) domain.ScopeIdentity {
	t.Helper()
	scope, err := domain.NewScopeIdentity("meigma/example", "secrets", "/acme/payments")
	require.NoError(t, err)

	return scope
}

// ownedApplicationEvidence returns an exact owned current desired value.
func ownedApplicationEvidence(desired domain.DesiredSecret, scope domain.ScopeIdentity) domain.ObservedEvidence {
	value := string(desired.Value().CopyCanonicalJSON())

	return ownedApplicationEvidenceWithValue(desired, scope, value)
}

// ownedApplicationEvidenceWithValue returns exact owned evidence with a chosen current value.
func ownedApplicationEvidenceWithValue(
	desired domain.DesiredSecret,
	scope domain.ScopeIdentity,
	value string,
) domain.ObservedEvidence {
	return domain.ObservedEvidence{
		Exists: true,
		ReservedTags: map[string]string{
			domain.ManagedByTagKey: domain.ManagedByTagValue,
			domain.ScopeTagKey:     scope.Value(),
			domain.SourceTagKey:    desired.Source().Value(),
		},
		StagingValid: true, CurrentVersion: "version-1", SecretString: &value,
	}
}

var _ error = ambiguousMutationError{}
