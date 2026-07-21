package gitrepo_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/sops-aws-sync/internal/adapters/gitrepo"
	"github.com/meigma/sops-aws-sync/internal/domain"
)

// TestReaderUsesExactCommitRatherThanWorkingTree proves committed desired-state authority.
func TestReaderUsesExactCommitRatherThanWorkingTree(t *testing.T) {
	committed := []byte(`{"committed":true}`)
	repositoryPath, revision := createRepository(t, "secrets/nested/value.sops.json", committed)
	require.NoError(t, os.WriteFile(filepath.Join(repositoryPath, "secrets/nested/value.sops.json"),
		[]byte(`{"working":true}`), 0o600))
	reader, err := gitrepo.NewReader(repositoryPath, 1024)
	require.NoError(t, err)

	snapshot, err := reader.Load(context.Background(), revision, "secrets")
	require.NoError(t, err)
	require.Len(t, snapshot.Documents, 1)
	assert.Equal(t, "secrets/nested/value.sops.json", snapshot.Documents[0].Path)
	assert.Equal(t, domain.SourceFormatJSON, snapshot.Documents[0].Format)
	assert.Equal(t, committed, snapshot.Documents[0].Data)
	assert.Equal(t, revision, snapshot.Revision.Value())
}

// TestReaderSelectsSupportedSourceFormats proves both YAML suffixes carry one encoding contract.
func TestReaderSelectsSupportedSourceFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		wantFormat domain.SourceFormat
	}{
		{name: "JSON", path: "secrets/value.sops.json", wantFormat: domain.SourceFormatJSON},
		{name: "YAML", path: "secrets/value.sops.yaml", wantFormat: domain.SourceFormatYAML},
		{name: "YML", path: "secrets/value.sops.yml", wantFormat: domain.SourceFormatYAML},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repositoryPath, revision := createRepository(t, test.path, []byte("encrypted"))
			reader, err := gitrepo.NewReader(repositoryPath, 1024)
			require.NoError(t, err)

			snapshot, err := reader.Load(context.Background(), revision, "secrets")
			require.NoError(t, err)
			require.Len(t, snapshot.Documents, 1)
			assert.Equal(t, test.path, snapshot.Documents[0].Path)
			assert.Equal(t, test.wantFormat, snapshot.Documents[0].Format)
		})
	}
}

// TestReaderRejectsMatchingSymlink proves selected non-regular entries fail desired construction.
func TestReaderRejectsMatchingSymlink(t *testing.T) {
	repositoryPath := t.TempDir()
	repository, err := git.PlainInit(repositoryPath, false)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(repositoryPath, "secrets"), 0o750))
	require.NoError(t, os.Symlink("target.yaml", filepath.Join(repositoryPath, "secrets/value.sops.yaml")))
	worktree, err := repository.Worktree()
	require.NoError(t, err)
	_, err = worktree.Add("secrets/value.sops.yaml")
	require.NoError(t, err)
	revision, err := worktree.Commit("test: add symlink", &git.CommitOptions{Author: testSignature()})
	require.NoError(t, err)
	reader, err := gitrepo.NewReader(repositoryPath, 1024)
	require.NoError(t, err)

	_, err = reader.Load(context.Background(), revision.String(), "secrets")
	require.ErrorContains(t, err, "not a regular Git blob")
}

// TestReaderRejectsMatchingSubmodule proves gitlinks cannot silently remove desired state.
func TestReaderRejectsMatchingSubmodule(t *testing.T) {
	t.Parallel()

	repositoryPath := t.TempDir()
	repository, err := git.PlainInit(repositoryPath, false)
	require.NoError(t, err)
	require.NoError(t, repository.Storer.SetIndex(&index.Index{
		Version: 2,
		Entries: []*index.Entry{{
			Name: "secrets/value.sops.yml",
			Hash: plumbing.NewHash("0123456789abcdef0123456789abcdef01234567"),
			Mode: filemode.Submodule,
		}},
	}))
	worktree, err := repository.Worktree()
	require.NoError(t, err)
	revision, err := worktree.Commit("test: add submodule", &git.CommitOptions{Author: testSignature()})
	require.NoError(t, err)
	reader, err := gitrepo.NewReader(repositoryPath, 1024)
	require.NoError(t, err)

	_, err = reader.Load(context.Background(), revision.String(), "secrets")
	require.ErrorContains(t, err, "not a regular Git blob")
}

// createRepository creates one committed regular fixture repository.
func createRepository(t *testing.T, relativePath string, content []byte) (string, string) {
	t.Helper()
	repositoryPath := t.TempDir()
	repository, err := git.PlainInit(repositoryPath, false)
	require.NoError(t, err)
	absolutePath := filepath.Join(repositoryPath, relativePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(absolutePath), 0o750))
	require.NoError(t, os.WriteFile(absolutePath, content, 0o600))
	worktree, err := repository.Worktree()
	require.NoError(t, err)
	_, err = worktree.Add(relativePath)
	require.NoError(t, err)
	revision, err := worktree.Commit("test: add fixture", &git.CommitOptions{Author: testSignature()})
	require.NoError(t, err)

	return repositoryPath, revision.String()
}

// testSignature returns deterministic Git metadata for fixture commits.
func testSignature() *object.Signature {
	return &object.Signature{Name: "Test", Email: "test@example.invalid", When: time.Unix(1_784_565_712, 0).UTC()}
}
