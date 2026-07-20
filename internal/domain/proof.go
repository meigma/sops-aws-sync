package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
)

const (
	// sopsJSONSuffix identifies source documents selected by the V1 contract.
	sopsJSONSuffix = ".sops.json"
	// maximumSecretNameBytes is the AWS Secrets Manager name length limit.
	maximumSecretNameBytes = 512
)

// SecretName is a validated AWS Secrets Manager name.
type SecretName struct {
	value string
}

// NewSecretName validates and constructs an AWS Secrets Manager name.
func NewSecretName(value string) (SecretName, error) {
	if value == "" {
		return SecretName{}, errors.New("secret name is empty")
	}
	if len(value) > maximumSecretNameBytes {
		return SecretName{}, fmt.Errorf("secret name exceeds %d bytes", maximumSecretNameBytes)
	}
	if strings.IndexFunc(value, isUnsupportedSecretNameRune) >= 0 {
		return SecretName{}, errors.New("secret name contains an unsupported character")
	}

	return SecretName{value: value}, nil
}

// Value returns the validated name for use by an application adapter.
func (name SecretName) Value() string {
	return name.value
}

// isUnsupportedSecretNameRune reports whether AWS Secrets Manager rejects a rune.
func isUnsupportedSecretNameRune(value rune) bool {
	if value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' {
		return false
	}

	return !strings.ContainsRune("/_+=.@-", value)
}

// SourceIdentity identifies one repository-relative desired-state path.
type SourceIdentity struct {
	digest [sha256.Size]byte
}

// NewSourceIdentity derives the source identity from a normalized repository path.
func NewSourceIdentity(sourcePath string) (SourceIdentity, error) {
	cleaned, err := cleanRepositoryPath(sourcePath)
	if err != nil {
		return SourceIdentity{}, fmt.Errorf("source identity: %w", err)
	}

	return SourceIdentity{digest: sha256.Sum256([]byte(cleaned))}, nil
}

// Value returns the lowercase SHA-256 source digest.
func (identity SourceIdentity) Value() string {
	return hex.EncodeToString(identity.digest[:])
}

// Revision identifies the exact Git commit used to build desired state.
type Revision struct {
	value string
}

// NewRevision validates and constructs a full Git object identifier.
func NewRevision(value string) (Revision, error) {
	if len(value) != sha256.Size+8 {
		return Revision{}, errors.New("revision must be a full 40-character Git object identifier")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return Revision{}, fmt.Errorf("revision must be hexadecimal: %w", err)
	}

	return Revision{value: strings.ToLower(value)}, nil
}

// Value returns the exact Git object identifier.
func (revision Revision) Value() string {
	return revision.value
}

// SecretValue contains canonical JSON without generic formatting or encoding behavior.
type SecretValue struct {
	canonicalJSON []byte
}

// NewSecretValue constructs a secret value from already validated JCS bytes.
func NewSecretValue(canonicalJSON []byte) (SecretValue, error) {
	if len(canonicalJSON) == 0 {
		return SecretValue{}, errors.New("canonical JSON is empty")
	}

	return SecretValue{canonicalJSON: append([]byte(nil), canonicalJSON...)}, nil
}

// Equal reports whether two canonical secret values contain identical bytes.
func (value SecretValue) Equal(other SecretValue) bool {
	return bytes.Equal(value.canonicalJSON, other.canonicalJSON)
}

// CopyCanonicalJSON returns a defensive copy for a narrowly scoped adapter boundary.
func (value SecretValue) CopyCanonicalJSON() []byte {
	return append([]byte(nil), value.canonicalJSON...)
}

// DesiredSecret is one committed source document mapped to one AWS secret.
type DesiredSecret struct {
	name     SecretName
	source   SourceIdentity
	value    SecretValue
	revision Revision
}

// NewDesiredSecret constructs a desired secret from validated domain values.
func NewDesiredSecret(
	name SecretName,
	source SourceIdentity,
	value SecretValue,
	revision Revision,
) DesiredSecret {
	return DesiredSecret{
		name:     name,
		source:   source,
		value:    value,
		revision: revision,
	}
}

// Name returns the desired AWS secret identity.
func (secret DesiredSecret) Name() SecretName {
	return secret.name
}

// Source returns the desired source-path identity.
func (secret DesiredSecret) Source() SourceIdentity {
	return secret.source
}

// Value returns the canonical desired value.
func (secret DesiredSecret) Value() SecretValue {
	return secret.value
}

// Revision returns the exact desired Git revision.
func (secret DesiredSecret) Revision() Revision {
	return secret.revision
}

// DecisionKind identifies one pure reconciliation decision.
type DecisionKind string

const (
	// DecisionCreate creates a desired secret when the observed slot is absent.
	DecisionCreate DecisionKind = "create"
)

// Operation is one executable decision over a desired secret.
type Operation struct {
	kind    DecisionKind
	desired DesiredSecret
}

// Kind returns the operation decision.
func (operation Operation) Kind() DecisionKind {
	return operation.kind
}

// Desired returns the immutable desired secret for the operation.
func (operation Operation) Desired() DesiredSecret {
	return operation.desired
}

// Plan is a deterministic, secret-safe set of reconciliation operations.
type Plan struct {
	operations []Operation
}

// PlanCreates plans desired secrets against absent observed state.
func PlanCreates(desired []DesiredSecret) (Plan, error) {
	sorted := append([]DesiredSecret(nil), desired...)
	sort.Slice(sorted, func(left, right int) bool {
		return sorted[left].Name().Value() < sorted[right].Name().Value()
	})

	operations := make([]Operation, 0, len(sorted))
	for index, secret := range sorted {
		if index > 0 && sorted[index-1].Name() == secret.Name() {
			return Plan{}, errors.New("duplicate desired secret name")
		}
		operations = append(operations, Operation{kind: DecisionCreate, desired: secret})
	}

	return Plan{operations: operations}, nil
}

// Operations returns a defensive copy of the deterministic operations.
func (plan Plan) Operations() []Operation {
	return append([]Operation(nil), plan.operations...)
}

// PlanReport contains only non-sensitive Phase 1 proof evidence.
type PlanReport struct {
	// SchemaVersion identifies the disposable proof report shape.
	SchemaVersion string `json:"schema_version"`
	// CreateCount is the number of planned create operations.
	CreateCount int `json:"create_count"`
}

// Report returns a summary that omits secret values, names, and source paths.
func (plan Plan) Report() PlanReport {
	return PlanReport{
		SchemaVersion: "phase1-proof/v1",
		CreateCount:   len(plan.operations),
	}
}

// MapSourcePath maps one selected source path to a secret name and source identity.
func MapSourcePath(sourceRoot, secretPrefix, sourcePath string) (SecretName, SourceIdentity, error) {
	cleanRoot, err := cleanSourceRoot(sourceRoot)
	if err != nil {
		return SecretName{}, SourceIdentity{}, err
	}
	cleanSource, err := cleanRepositoryPath(sourcePath)
	if err != nil {
		return SecretName{}, SourceIdentity{}, err
	}

	relative, err := relativeSourcePath(cleanRoot, cleanSource)
	if err != nil {
		return SecretName{}, SourceIdentity{}, err
	}
	if !strings.HasSuffix(relative, sopsJSONSuffix) {
		return SecretName{}, SourceIdentity{}, fmt.Errorf("source path must end in %s", sopsJSONSuffix)
	}

	stem := strings.TrimSuffix(relative, sopsJSONSuffix)
	if stem == "" {
		return SecretName{}, SourceIdentity{}, errors.New("source path has an empty relative stem")
	}
	prefix := strings.TrimSuffix(secretPrefix, "/")
	if prefix == "" {
		return SecretName{}, SourceIdentity{}, errors.New("secret prefix is empty")
	}

	name, err := NewSecretName(prefix + "/" + stem)
	if err != nil {
		return SecretName{}, SourceIdentity{}, fmt.Errorf("map source path: %w", err)
	}
	identity, err := NewSourceIdentity(cleanSource)
	if err != nil {
		return SecretName{}, SourceIdentity{}, err
	}

	return name, identity, nil
}

// cleanSourceRoot validates a repository-relative source directory.
func cleanSourceRoot(sourceRoot string) (string, error) {
	if sourceRoot == "." {
		return sourceRoot, nil
	}

	return cleanRepositoryPath(sourceRoot)
}

// cleanRepositoryPath rejects paths whose normalization could change identity.
func cleanRepositoryPath(value string) (string, error) {
	if value == "" || path.IsAbs(value) {
		return "", errors.New("path must be non-empty and repository-relative")
	}
	cleaned := path.Clean(value)
	if cleaned != value || cleaned == "." || strings.HasPrefix(cleaned, "../") {
		return "", errors.New("path must already be normalized inside the repository")
	}

	return cleaned, nil
}

// relativeSourcePath removes the exact configured root from a selected path.
func relativeSourcePath(sourceRoot, sourcePath string) (string, error) {
	if sourceRoot == "." {
		return sourcePath, nil
	}
	prefix := sourceRoot + "/"
	if !strings.HasPrefix(sourcePath, prefix) {
		return "", errors.New("source path is outside the configured source root")
	}

	return strings.TrimPrefix(sourcePath, prefix), nil
}
