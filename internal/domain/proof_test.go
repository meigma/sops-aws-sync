package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/template-go/internal/domain"
)

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

func TestPlanCreatesIsDeterministicAndReportSafe(t *testing.T) {
	t.Parallel()

	desired := []domain.DesiredSecret{
		newDesiredSecret(t, "/acme/payments/z", `{"secret":"phase1-plaintext-sentinel"}`),
		newDesiredSecret(t, "/acme/payments/a", `{"secret":"another-sentinel"}`),
	}
	plan, err := domain.PlanCreates(desired)
	require.NoError(t, err)

	operations := plan.Operations()
	require.Len(t, operations, 2)
	assert.Equal(t, domain.DecisionCreate, operations[0].Kind())
	assert.Equal(t, "/acme/payments/a", operations[0].Desired().Name().Value())
	assert.Equal(t, "/acme/payments/z", operations[1].Desired().Name().Value())

	report, err := json.Marshal(plan.Report())
	require.NoError(t, err)
	assert.JSONEq(t, `{"schema_version":"phase1-proof/v1","create_count":2}`, string(report))
	assert.NotContains(t, string(report), "phase1-plaintext-sentinel")
	assert.NotContains(t, string(report), "/acme/payments")
}

func newDesiredSecret(t *testing.T, nameValue, canonicalJSON string) domain.DesiredSecret {
	t.Helper()

	name, err := domain.NewSecretName(nameValue)
	require.NoError(t, err)
	source, err := domain.NewSourceIdentity("secrets/production/database.sops.json")
	require.NoError(t, err)
	value, err := domain.NewSecretValue([]byte(canonicalJSON))
	require.NoError(t, err)
	revision, err := domain.NewRevision("0123456789abcdef0123456789abcdef01234567")
	require.NoError(t, err)

	return domain.NewDesiredSecret(name, source, value, revision)
}
