package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/meigma/template-go/internal/domain"
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

// SourceRepository loads committed encrypted documents without consulting the working tree.
type SourceRepository interface {
	Load(ctx context.Context, revision, sourceRoot string) (SourceSnapshot, error)
}

// JSONDecrypter converts one encrypted JSON document into a validated canonical value.
type JSONDecrypter interface {
	DecryptJSON(ctx context.Context, encrypted []byte) (domain.SecretValue, error)
}

// ProofInput configures the disposable committed-source planning proof.
type ProofInput struct {
	// Revision is the commit-ish resolved by the source repository.
	Revision string
	// SourceRoot selects repository-relative SOPS JSON documents.
	SourceRoot string
	// SecretPrefix is joined to each selected relative source stem.
	SecretPrefix string
}

// BuildCommittedSourcePlan proves one committed source document becomes one create operation.
func BuildCommittedSourcePlan(
	ctx context.Context,
	sourceRepository SourceRepository,
	decrypter JSONDecrypter,
	input ProofInput,
) (domain.Plan, error) {
	if sourceRepository == nil || decrypter == nil {
		return domain.Plan{}, errors.New("proof adapters are required")
	}
	if err := ctx.Err(); err != nil {
		return domain.Plan{}, fmt.Errorf("build committed source plan: %w", err)
	}

	snapshot, err := sourceRepository.Load(ctx, input.Revision, input.SourceRoot)
	if err != nil {
		return domain.Plan{}, fmt.Errorf("load committed source: %w", err)
	}
	if len(snapshot.Documents) != 1 {
		return domain.Plan{}, fmt.Errorf(
			"phase 1 proof requires exactly one selected document, got %d",
			len(snapshot.Documents),
		)
	}

	document := snapshot.Documents[0]
	value, err := decrypter.DecryptJSON(ctx, document.Data)
	if err != nil {
		return domain.Plan{}, fmt.Errorf("decrypt committed source: %w", err)
	}
	name, source, err := domain.MapSourcePath(input.SourceRoot, input.SecretPrefix, document.Path)
	if err != nil {
		return domain.Plan{}, fmt.Errorf("map committed source: %w", err)
	}

	desired := domain.NewDesiredSecret(name, source, value, snapshot.Revision)
	plan, err := domain.PlanCreates([]domain.DesiredSecret{desired})
	if err != nil {
		return domain.Plan{}, fmt.Errorf("plan committed source: %w", err)
	}

	return plan, nil
}
