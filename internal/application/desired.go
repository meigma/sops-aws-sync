package application

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/meigma/sops-aws-sync/internal/domain"
)

// EncryptedDocument is one selected encrypted Git blob at a repository path.
type EncryptedDocument struct {
	// Path is the normalized repository-relative source path.
	Path string
	// Format identifies the source encoding selected from the path suffix.
	Format domain.SourceFormat
	// Data is an isolated copy of the encrypted Git blob.
	Data []byte
}

// SourceSnapshot contains selected documents from one exact Git commit.
type SourceSnapshot struct {
	// Revision is the exact commit loaded by the source adapter.
	Revision domain.Revision
	// Documents are the selected regular SOPS JSON or YAML blobs.
	Documents []EncryptedDocument
}

// SourceRepository loads committed documents without consulting the working tree.
type SourceRepository interface {
	Load(ctx context.Context, revision, sourceRoot string) (SourceSnapshot, error)
}

// DocumentDecrypter converts an encrypted source document into a validated canonical value.
type DocumentDecrypter interface {
	Decrypt(ctx context.Context, format domain.SourceFormat, encrypted []byte) (domain.SecretValue, error)
}

// DesiredInput configures committed desired-state construction.
type DesiredInput struct {
	// Revision is the commit-ish resolved by the source repository.
	Revision string
	// SourceRoot selects repository-relative SOPS JSON or YAML documents.
	SourceRoot string
	// SecretPrefix is joined to each selected relative source stem.
	SecretPrefix string
}

// DesiredSnapshot is the complete desired state from one exact committed revision.
type DesiredSnapshot struct {
	secrets  []domain.DesiredSecret
	revision domain.Revision
}

// Revision returns the exact committed revision behind the desired snapshot.
func (snapshot DesiredSnapshot) Revision() domain.Revision {
	return snapshot.revision
}

// copySecrets returns an isolated slice of immutable desired values.
func (snapshot DesiredSnapshot) copySecrets() []domain.DesiredSecret {
	return append([]domain.DesiredSecret(nil), snapshot.secrets...)
}

// BuildDesiredSnapshot constructs the complete immutable desired snapshot.
func BuildDesiredSnapshot(
	ctx context.Context,
	sourceRepository SourceRepository,
	decrypter DocumentDecrypter,
	input DesiredInput,
) (DesiredSnapshot, error) {
	if sourceRepository == nil || decrypter == nil {
		return DesiredSnapshot{}, errors.New("desired-state adapters are required")
	}
	if err := ctx.Err(); err != nil {
		return DesiredSnapshot{}, fmt.Errorf("build desired snapshot: %w", err)
	}
	snapshot, err := sourceRepository.Load(ctx, input.Revision, input.SourceRoot)
	if err != nil {
		return DesiredSnapshot{}, fmt.Errorf("load committed source: %w", err)
	}
	desired := make([]domain.DesiredSecret, 0, len(snapshot.Documents))
	for _, document := range snapshot.Documents {
		value, decryptErr := decrypter.Decrypt(ctx, document.Format, document.Data)
		if decryptErr != nil {
			return DesiredSnapshot{}, fmt.Errorf("decrypt committed source: %w", decryptErr)
		}
		name, source, mapErr := domain.MapSourcePath(input.SourceRoot, input.SecretPrefix, document.Path)
		if mapErr != nil {
			return DesiredSnapshot{}, fmt.Errorf("map committed source: %w", mapErr)
		}
		desired = append(desired, domain.NewDesiredSecret(name, source, value, snapshot.Revision))
	}
	sort.Slice(desired, func(left, right int) bool {
		return desired[left].Name().Value() < desired[right].Name().Value()
	})
	for index := 1; index < len(desired); index++ {
		if desired[index-1].Name() == desired[index].Name() {
			return DesiredSnapshot{}, errors.New("multiple source documents map to the same secret")
		}
	}

	return DesiredSnapshot{secrets: desired, revision: snapshot.Revision}, nil
}
