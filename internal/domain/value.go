package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"
)

const (
	sopsJSONSuffix         = ".sops.json"
	maximumSecretNameSize  = 512
	maximumSecretValueSize = 65_536
	gitSHA1HexLength       = 40
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
	if len(value) > maximumSecretNameSize {
		return SecretName{}, fmt.Errorf("secret name exceeds %d bytes", maximumSecretNameSize)
	}
	if strings.IndexFunc(value, isUnsupportedSecretNameRune) >= 0 {
		return SecretName{}, errors.New("secret name contains an unsupported character")
	}

	return SecretName{value: value}, nil
}

// Value returns the validated name for an application adapter.
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

// SourceIdentity identifies one normalized repository-relative desired-state path.
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

// ParseSourceIdentity validates a lowercase or uppercase SHA-256 source tag.
func ParseSourceIdentity(value string) (SourceIdentity, error) {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return SourceIdentity{}, errors.New("source identity must be a SHA-256 digest")
	}
	var digest [sha256.Size]byte
	copy(digest[:], decoded)

	return SourceIdentity{digest: digest}, nil
}

// Value returns the lowercase SHA-256 source digest.
func (identity SourceIdentity) Value() string {
	return hex.EncodeToString(identity.digest[:])
}

// ScopeIdentity identifies one repository, source root, and AWS prefix ownership scope.
type ScopeIdentity struct {
	digest [sha256.Size]byte
}

// NewScopeIdentity derives the versioned ownership scope identifier.
func NewScopeIdentity(repositoryID, sourceRoot, secretPrefix string) (ScopeIdentity, error) {
	if strings.TrimSpace(repositoryID) == "" {
		return ScopeIdentity{}, errors.New("repository identity is empty")
	}
	cleanRoot, err := cleanSourceRoot(sourceRoot)
	if err != nil {
		return ScopeIdentity{}, fmt.Errorf("scope source root: %w", err)
	}
	prefix := strings.TrimSuffix(secretPrefix, "/")
	if _, err := NewSecretName(prefix); err != nil {
		return ScopeIdentity{}, fmt.Errorf("scope secret prefix: %w", err)
	}
	material := "sops-aws-sync:scope:v1\x00" + repositoryID + "\x00" + cleanRoot + "\x00" + prefix

	return ScopeIdentity{digest: sha256.Sum256([]byte(material))}, nil
}

// Value returns the lowercase SHA-256 scope digest.
func (identity ScopeIdentity) Value() string {
	return hex.EncodeToString(identity.digest[:])
}

// Revision identifies the exact Git commit used to build desired state.
type Revision struct {
	value string
}

// NewRevision validates and constructs a full Git SHA-1 object identifier.
func NewRevision(value string) (Revision, error) {
	if len(value) != gitSHA1HexLength {
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

// SecretValue contains canonical JSON without generic formatting behavior.
type SecretValue struct {
	canonicalJSON []byte
}

// NewSecretValue constructs a secret value from validated RFC 8785 bytes.
func NewSecretValue(canonicalJSON []byte) (SecretValue, error) {
	if len(canonicalJSON) == 0 {
		return SecretValue{}, errors.New("canonical JSON is empty")
	}
	if len(canonicalJSON) > maximumSecretValueSize {
		return SecretValue{}, fmt.Errorf("canonical JSON exceeds %d bytes", maximumSecretValueSize)
	}

	return SecretValue{canonicalJSON: append([]byte(nil), canonicalJSON...)}, nil
}

// Equal reports whether two canonical secret values contain identical bytes.
func (value SecretValue) Equal(other SecretValue) bool {
	return bytes.Equal(value.canonicalJSON, other.canonicalJSON)
}

// EqualString reports whether the canonical value exactly matches an observed SecretString.
func (value SecretValue) EqualString(observed string) bool {
	return bytes.Equal(value.canonicalJSON, []byte(observed))
}

// CopyCanonicalJSON returns a defensive copy for the Secrets Manager adapter.
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

// NewDesiredSecret constructs a desired secret from validated values.
func NewDesiredSecret(name SecretName, source SourceIdentity, value SecretValue, revision Revision) DesiredSecret {
	return DesiredSecret{name: name, source: source, value: value, revision: revision}
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

// MapSourcePath maps one selected source path to a name and source identity.
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
