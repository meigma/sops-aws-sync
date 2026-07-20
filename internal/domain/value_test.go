package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/sops-aws-sync/internal/domain"
)

// TestMapSourcePath proves deterministic name and source identity derivation.
func TestMapSourcePath(t *testing.T) {
	t.Parallel()

	name, source, err := domain.MapSourcePath(
		"secrets",
		"/acme/payments",
		"secrets/production/database.sops.json",
	)
	require.NoError(t, err)
	assert.Equal(t, "/acme/payments/production/database", name.Value())
	assert.Len(t, source.Value(), 64)
}

// TestClassifyDirectCoversDesiredNameSafetyStates proves the Phase 2 classifier fails closed.
func TestClassifyDirectCoversDesiredNameSafetyStates(t *testing.T) {
	t.Parallel()

	desired := newDesiredSecret(t, "/acme/payments/database", `{"password":"sentinel"}`)
	scope := newScope(t)
	equal := string(desired.Value().CopyCanonicalJSON())
	tests := []struct {
		name     string
		evidence domain.ObservedEvidence
		want     domain.ObservedKind
	}{
		{name: "missing", evidence: domain.ObservedEvidence{}, want: domain.ObservedMissing},
		{name: "foreign tags", evidence: domain.ObservedEvidence{Exists: true}, want: domain.ObservedForeign},
		{
			name:     "invalid source tag",
			evidence: ownedEvidence(scope, desired, &equal, false),
			want:     domain.ObservedOwnedActiveString,
		},
		{name: "binary", evidence: ownedEvidence(scope, desired, nil, true), want: domain.ObservedOwnedActiveBinary},
		{
			name:     "without current",
			evidence: ownedEvidence(scope, desired, nil, false),
			want:     domain.ObservedOwnedWithoutCurrent,
		},
		{name: "scheduled", evidence: scheduledEvidence(scope, desired), want: domain.ObservedOwnedScheduled},
		{
			name:     "service owned",
			evidence: constrainedEvidence(scope, desired, "service"),
			want:     domain.ObservedConflict,
		},
		{name: "rotation", evidence: constrainedEvidence(scope, desired, "rotation"), want: domain.ObservedConflict},
		{name: "replica", evidence: constrainedEvidence(scope, desired, "replica"), want: domain.ObservedConflict},
		{
			name:     "ambiguous staging",
			evidence: constrainedEvidence(scope, desired, "staging"),
			want:     domain.ObservedConflict,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := domain.ClassifyDirect(desired, scope, test.evidence)
			assert.Equal(t, test.want, got.Kind())
		})
	}
}

// TestBuildPlanAndTransitions proves deterministic create, update, no-op, restore, and preconditions.
func TestBuildPlanAndTransitions(t *testing.T) {
	t.Parallel()

	scope := newScope(t)
	createDesired := newDesiredSecret(t, "/acme/payments/a", `{"value":"a"}`)
	updateDesired := newDesiredSecret(t, "/acme/payments/b", `{"value":"b"}`)
	noOpDesired := newDesiredSecret(t, "/acme/payments/c", `{"value":"c"}`)
	restoreDesired := newDesiredSecret(t, "/acme/payments/d", `{"value":"d"}`)
	old := `{"value":"old"}`
	equal := string(noOpDesired.Value().CopyCanonicalJSON())
	desired := []domain.DesiredSecret{restoreDesired, noOpDesired, updateDesired, createDesired}
	observed := []domain.ObservedSlot{
		domain.ClassifyDirect(createDesired, scope, domain.ObservedEvidence{}),
		domain.ClassifyDirect(updateDesired, scope, ownedEvidence(scope, updateDesired, &old, false)),
		domain.ClassifyDirect(noOpDesired, scope, ownedEvidence(scope, noOpDesired, &equal, false)),
		domain.ClassifyDirect(restoreDesired, scope, scheduledEvidence(scope, restoreDesired)),
	}
	plan, err := domain.BuildPlan(desired, observed)
	require.NoError(t, err)
	operations := plan.Operations()
	require.Len(t, operations, 3)
	assert.Equal(t, domain.DecisionRestore, operations[0].Kind())
	assert.Equal(t, domain.DecisionCreate, operations[1].Kind())
	assert.Equal(t, domain.DecisionUpdate, operations[2].Kind())
	assert.Equal(t, 1, plan.UnchangedCount())
	require.Error(t, plan.ValidatePhaseTwo())

	createOperation := operations[1]
	assert.Equal(t, domain.TransitionApply, domain.CheckPrecondition(createOperation, observed[0]))
	assert.Equal(t, domain.TransitionRetrySameToken, domain.ResolveAmbiguous(createOperation, observed[0]))
	createdValue := string(createDesired.Value().CopyCanonicalJSON())
	created := domain.ClassifyDirect(createDesired, scope, ownedEvidence(scope, createDesired, &createdValue, false))
	assert.Equal(t, domain.TransitionSucceeded, domain.ResolveAmbiguous(createOperation, created))
}

// newDesiredSecret builds one validated test desired value.
func newDesiredSecret(t *testing.T, nameValue, canonicalJSON string) domain.DesiredSecret {
	t.Helper()
	name, err := domain.NewSecretName(nameValue)
	require.NoError(t, err)
	source, err := domain.NewSourceIdentity("secrets/" + nameValue[len("/acme/payments/"):] + ".sops.json")
	require.NoError(t, err)
	value, err := domain.NewSecretValue([]byte(canonicalJSON))
	require.NoError(t, err)
	revision, err := domain.NewRevision("0123456789abcdef0123456789abcdef01234567")
	require.NoError(t, err)

	return domain.NewDesiredSecret(name, source, value, revision)
}

// newScope builds the stable test ownership scope.
func newScope(t *testing.T) domain.ScopeIdentity {
	t.Helper()
	scope, err := domain.NewScopeIdentity("meigma/example", "secrets", "/acme/payments")
	require.NoError(t, err)

	return scope
}

// ownedEvidence returns exact owned active evidence for one desired name.
func ownedEvidence(
	scope domain.ScopeIdentity,
	desired domain.DesiredSecret,
	secretString *string,
	binary bool,
) domain.ObservedEvidence {
	return domain.ObservedEvidence{
		Exists: true,
		ReservedTags: map[string]string{
			domain.ManagedByTagKey: domain.ManagedByTagValue,
			domain.ScopeTagKey:     scope.Value(),
			domain.SourceTagKey:    desired.Source().Value(),
		},
		StagingValid:   true,
		CurrentVersion: "version-1",
		SecretString:   secretString,
		SecretBinary:   binary,
	}
}

// scheduledEvidence returns exact owned scheduled evidence.
func scheduledEvidence(scope domain.ScopeIdentity, desired domain.DesiredSecret) domain.ObservedEvidence {
	evidence := ownedEvidence(scope, desired, nil, false)
	evidence.ScheduledForDeletion = true

	return evidence
}

// constrainedEvidence returns one exact owned but unsafe observed constraint.
func constrainedEvidence(
	scope domain.ScopeIdentity,
	desired domain.DesiredSecret,
	constraint string,
) domain.ObservedEvidence {
	evidence := ownedEvidence(scope, desired, nil, false)
	switch constraint {
	case "service":
		evidence.OwningService = "provider.example"
	case "rotation":
		evidence.RotationEnabled = true
	case "replica":
		evidence.HasReplicas = true
	case "staging":
		evidence.StagingValid = false
	}

	return evidence
}
