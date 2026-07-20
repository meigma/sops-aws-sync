package application_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/template-go/internal/adapters/gitrepo"
	"github.com/meigma/template-go/internal/adapters/sopsdecrypt"
	"github.com/meigma/template-go/internal/application"
	"github.com/meigma/template-go/internal/domain"
)

const (
	fixtureRelativePath = "secrets/production/database.sops.json"
	fixturePlaintext    = "phase1-plaintext-sentinel"
	maxEncryptedBytes   = 8 * 1024 * 1024
	maxCanonicalBytes   = 65_536
)

func TestCommittedSourcePlanningProof(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", fixtureRelativePath))
	require.NoError(t, err)
	repositoryPath, revision := createCommittedFixtureRepository(t, fixture)

	workingTreeSentinel := []byte(`{"credential":"working-tree-sentinel"}`)
	require.NoError(t, os.WriteFile(filepath.Join(repositoryPath, fixtureRelativePath), workingTreeSentinel, 0o600))
	t.Setenv(
		"SOPS_AGE_KEY",
		"AGE-SECRET-KEY-"+"1G0Q5K9TV4REQ3ZSQRMTMG8NSWQGYT0T7TZ33RAZEE0GZYVZN0APSU24RK7",
	)

	sourceReader, err := gitrepo.NewReader(repositoryPath, maxEncryptedBytes)
	require.NoError(t, err)
	decrypter, err := sopsdecrypt.New(maxEncryptedBytes, maxCanonicalBytes)
	require.NoError(t, err)
	plan, err := application.BuildCommittedSourcePlan(
		context.Background(),
		sourceReader,
		decrypter,
		application.ProofInput{
			Revision:     revision,
			SourceRoot:   "secrets",
			SecretPrefix: "/acme/payments",
		},
	)
	require.NoError(t, err)

	operations := plan.Operations()
	require.Len(t, operations, 1)
	operation := operations[0]
	assert.Equal(t, domain.DecisionCreate, operation.Kind())
	assert.Equal(t, "/acme/payments/production/database", operation.Desired().Name().Value())
	assert.Equal(t, revision, operation.Desired().Revision().Value())
	assert.JSONEq(
		t,
		`{"credential":"phase1-plaintext-sentinel","nested":{"a":"value","b":true},"z":1}`,
		string(operation.Desired().Value().CopyCanonicalJSON()),
	)
	assert.NotContains(t, string(operation.Desired().Value().CopyCanonicalJSON()), "working-tree-sentinel")

	report, err := json.Marshal(plan.Report())
	require.NoError(t, err)
	assert.JSONEq(t, `{"schema_version":"phase1-proof/v1","create_count":1}`, string(report))
	assert.NotContains(t, string(report), fixturePlaintext)
	assert.NotContains(t, string(report), "database")
}

func TestCommittedSourcePlanningProofRejectsTamperedMAC(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", fixtureRelativePath))
	require.NoError(t, err)
	repositoryPath, revision := createCommittedFixtureRepository(t, tamperFixtureMAC(t, fixture))
	t.Setenv(
		"SOPS_AGE_KEY",
		"AGE-SECRET-KEY-"+"1G0Q5K9TV4REQ3ZSQRMTMG8NSWQGYT0T7TZ33RAZEE0GZYVZN0APSU24RK7",
	)

	sourceReader, err := gitrepo.NewReader(repositoryPath, maxEncryptedBytes)
	require.NoError(t, err)
	decrypter, err := sopsdecrypt.New(maxEncryptedBytes, maxCanonicalBytes)
	require.NoError(t, err)
	_, err = application.BuildCommittedSourcePlan(
		context.Background(),
		sourceReader,
		decrypter,
		application.ProofInput{
			Revision:     revision,
			SourceRoot:   "secrets",
			SecretPrefix: "/acme/payments",
		},
	)
	require.ErrorContains(t, err, "decrypt committed source")
}

func createCommittedFixtureRepository(t *testing.T, fixture []byte) (string, string) {
	t.Helper()

	repositoryPath := t.TempDir()
	repository, err := git.PlainInit(repositoryPath, false)
	require.NoError(t, err)
	fixturePath := filepath.Join(repositoryPath, fixtureRelativePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(fixturePath), 0o750))
	require.NoError(t, os.WriteFile(fixturePath, fixture, 0o600))
	worktree, err := repository.Worktree()
	require.NoError(t, err)
	_, err = worktree.Add(fixtureRelativePath)
	require.NoError(t, err)
	commit, err := worktree.Commit("test: add committed SOPS fixture", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Phase 1 Test",
			Email: "phase1@example.invalid",
			When:  time.Unix(1_784_565_712, 0).UTC(),
		},
	})
	require.NoError(t, err)

	return repositoryPath, commit.String()
}

func tamperFixtureMAC(t *testing.T, fixture []byte) []byte {
	t.Helper()

	var document map[string]any
	require.NoError(t, json.Unmarshal(fixture, &document))
	metadata, ok := document["sops"].(map[string]any)
	require.True(t, ok)
	mac, ok := metadata["mac"].(string)
	require.True(t, ok)
	marker := "data:"
	dataIndex := strings.Index(mac, marker)
	require.NotEqual(t, -1, dataIndex)
	characterIndex := dataIndex + len(marker)
	require.Less(t, characterIndex, len(mac))
	replacement := byte('A')
	if mac[characterIndex] == replacement {
		replacement = 'B'
	}
	metadata["mac"] = mac[:characterIndex] + string(replacement) + mac[characterIndex+1:]
	tampered, err := json.Marshal(document)
	require.NoError(t, err)

	return tampered
}
