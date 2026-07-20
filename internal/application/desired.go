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
	// Data is an isolated copy of the encrypted Git blob.
	Data []byte
}

// SourceSnapshot contains selected documents from one exact Git commit.
type SourceSnapshot struct {
	// Revision is the exact commit loaded by the source adapter.
	Revision domain.Revision
	// Documents are the selected regular SOPS JSON blobs.
	Documents []EncryptedDocument
}

// SourceRepository loads committed documents without consulting the working tree.
type SourceRepository interface {
	Load(ctx context.Context, revision, sourceRoot string) (SourceSnapshot, error)
}

// JSONDecrypter converts encrypted JSON into a validated canonical value.
type JSONDecrypter interface {
	DecryptJSON(ctx context.Context, encrypted []byte) (domain.SecretValue, error)
}

// DesiredInput configures committed desired-state construction.
type DesiredInput struct {
	// Revision is the commit-ish resolved by the source repository.
	Revision string
	// SourceRoot selects repository-relative SOPS JSON documents.
	SourceRoot string
	// SecretPrefix is joined to each selected relative source stem.
	SecretPrefix string
}

// BuildDesiredSnapshot constructs the complete immutable desired snapshot.
func BuildDesiredSnapshot(
	ctx context.Context,
	sourceRepository SourceRepository,
	decrypter JSONDecrypter,
	input DesiredInput,
) ([]domain.DesiredSecret, domain.Revision, error) {
	if sourceRepository == nil || decrypter == nil {
		return nil, domain.Revision{}, errors.New("desired-state adapters are required")
	}
	if err := ctx.Err(); err != nil {
		return nil, domain.Revision{}, fmt.Errorf("build desired snapshot: %w", err)
	}
	snapshot, err := sourceRepository.Load(ctx, input.Revision, input.SourceRoot)
	if err != nil {
		return nil, domain.Revision{}, fmt.Errorf("load committed source: %w", err)
	}
	desired := make([]domain.DesiredSecret, 0, len(snapshot.Documents))
	for _, document := range snapshot.Documents {
		value, decryptErr := decrypter.DecryptJSON(ctx, document.Data)
		if decryptErr != nil {
			return nil, domain.Revision{}, fmt.Errorf("decrypt committed source: %w", decryptErr)
		}
		name, source, mapErr := domain.MapSourcePath(input.SourceRoot, input.SecretPrefix, document.Path)
		if mapErr != nil {
			return nil, domain.Revision{}, fmt.Errorf("map committed source: %w", mapErr)
		}
		desired = append(desired, domain.NewDesiredSecret(name, source, value, snapshot.Revision))
	}
	sort.Slice(desired, func(left, right int) bool {
		return desired[left].Name().Value() < desired[right].Name().Value()
	})
	for index := 1; index < len(desired); index++ {
		if desired[index-1].Name() == desired[index].Name() {
			return nil, domain.Revision{}, errors.New("multiple source documents map to the same secret")
		}
	}

	return desired, snapshot.Revision, nil
}
