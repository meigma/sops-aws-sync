package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	// ManagedByTagKey is the reserved ownership marker key.
	ManagedByTagKey = "sops-aws-sync:managed-by"
	// ScopeTagKey is the reserved ownership scope key.
	ScopeTagKey = "sops-aws-sync:scope"
	// SourceTagKey is the reserved committed source identity key.
	SourceTagKey = "sops-aws-sync:source"
	// ManagedByTagValue is the exact reserved ownership marker value.
	ManagedByTagValue = "sops-aws-sync"
)

// ObservedEvidence is infrastructure-neutral evidence returned by direct observation.
type ObservedEvidence struct {
	// Exists reports whether DescribeSecret found the requested name.
	Exists bool
	// ScheduledForDeletion reports whether the secret has a deletion date.
	ScheduledForDeletion bool
	// ReservedTags contains only the three reserved ownership tag values.
	ReservedTags map[string]string
	// OwningService is non-empty for a service-owned secret.
	OwningService string
	// RotationEnabled reports whether rotation is enabled.
	RotationEnabled bool
	// RotationInProgress reports whether rotation metadata indicates an unfinished rotation.
	RotationInProgress bool
	// HasReplicas reports whether the primary secret has replica Regions.
	HasReplicas bool
	// StagingValid reports whether exactly one AWSCURRENT version is unambiguous.
	StagingValid bool
	// CurrentVersion identifies the unique AWSCURRENT version when present.
	CurrentVersion string
	// SecretString is the current string payload when present.
	SecretString *string
	// SecretBinary reports that the current payload is binary rather than a string.
	SecretBinary bool
}

// DiscoveryEvidence is one paginated candidate name and its normalized list metadata.
type DiscoveryEvidence struct {
	// Name is the validated AWS secret name returned by discovery.
	Name SecretName
	// Evidence contains normalized metadata used only for exact scope filtering.
	Evidence ObservedEvidence
}

// NewDiscoveryEvidence validates one discovered AWS name and pairs it with list metadata.
func NewDiscoveryEvidence(name string, evidence ObservedEvidence) (DiscoveryEvidence, error) {
	validated, err := NewSecretName(name)
	if err != nil {
		return DiscoveryEvidence{}, err
	}

	return DiscoveryEvidence{Name: validated, Evidence: evidence}, nil
}

// ObservedKind identifies a closed directly observed state.
type ObservedKind string

const (
	// ObservedMissing is an absent desired name.
	ObservedMissing ObservedKind = "missing"
	// ObservedForeign is a same-name secret outside the exact ownership boundary.
	ObservedForeign ObservedKind = "foreign"
	// ObservedOwnedActiveString is an owned active secret with a current string value.
	ObservedOwnedActiveString ObservedKind = "owned-active-string"
	// ObservedOwnedActiveUnknown is an owned active discovered secret whose payload was not needed.
	ObservedOwnedActiveUnknown ObservedKind = "owned-active-unknown"
	// ObservedOwnedActiveBinary is an owned active secret with a current binary value.
	ObservedOwnedActiveBinary ObservedKind = "owned-active-binary"
	// ObservedOwnedWithoutCurrent is an owned active secret without an AWSCURRENT value.
	ObservedOwnedWithoutCurrent ObservedKind = "owned-without-current"
	// ObservedOwnedScheduled is an owned secret scheduled for deletion.
	ObservedOwnedScheduled ObservedKind = "owned-scheduled"
	// ObservedConflict is owned evidence with an unsupported safety constraint.
	ObservedConflict ObservedKind = "conflict"
	// ObservedInvalid is malformed or internally inconsistent evidence.
	ObservedInvalid ObservedKind = "invalid"
)

// ObservedSlot is one classified desired-name state.
type ObservedSlot struct {
	name          SecretName
	kind          ObservedKind
	source        SourceIdentity
	secretString  string
	fingerprint   string
	conflictClass string
}

// ClassifyDirect classifies normalized direct-observation evidence in the pure domain.
func ClassifyDirect(
	desired DesiredSecret,
	scope ScopeIdentity,
	evidence ObservedEvidence,
) ObservedSlot {
	source := desired.Source()
	metadata, terminal := classifyMetadata(desired.Name(), &source, scope, evidence)
	if terminal {
		return metadata
	}

	name := desired.Name()
	if evidence.SecretBinary {
		if evidence.SecretString != nil {
			return newObservedSlot(name, ObservedInvalid, source, "", evidence.CurrentVersion, "payload")
		}
		return newObservedSlot(name, ObservedOwnedActiveBinary, source, "", evidence.CurrentVersion, "")
	}
	if evidence.SecretString == nil {
		return newObservedSlot(name, ObservedInvalid, source, "", evidence.CurrentVersion, "payload")
	}

	return newObservedSlot(name, ObservedOwnedActiveString, source, *evidence.SecretString, evidence.CurrentVersion, "")
}

// CurrentPayloadRequired reports whether metadata proves a payload read is owned and safe.
func CurrentPayloadRequired(
	desired DesiredSecret,
	scope ScopeIdentity,
	evidence ObservedEvidence,
) bool {
	source := desired.Source()
	_, terminal := classifyMetadata(desired.Name(), &source, scope, evidence)

	return !terminal
}

// IsScopeCandidate reports whether discovery evidence has the exact managed-by and scope tags.
func IsScopeCandidate(scope ScopeIdentity, evidence ObservedEvidence) bool {
	return evidence.Exists && evidence.ReservedTags != nil &&
		evidence.ReservedTags[ManagedByTagKey] == ManagedByTagValue &&
		evidence.ReservedTags[ScopeTagKey] == scope.Value()
}

// ClassifyManaged classifies a directly observed discovery candidate without reading its payload.
func ClassifyManaged(name SecretName, scope ScopeIdentity, evidence ObservedEvidence) ObservedSlot {
	metadata, terminal := classifyMetadata(name, nil, scope, evidence)
	if terminal {
		return metadata
	}

	return newObservedSlot(
		name,
		ObservedOwnedActiveUnknown,
		metadata.source,
		"",
		evidence.CurrentVersion,
		"",
	)
}

// classifyMetadata returns every state decidable without loading AWSCURRENT.
func classifyMetadata(
	name SecretName,
	expectedSource *SourceIdentity,
	scope ScopeIdentity,
	evidence ObservedEvidence,
) (ObservedSlot, bool) {
	if !evidence.Exists {
		return newObservedSlot(name, ObservedMissing, SourceIdentity{}, "", "", ""), true
	}
	if evidence.ReservedTags == nil {
		return newObservedSlot(name, ObservedForeign, SourceIdentity{}, "", evidence.CurrentVersion, "ownership"), true
	}
	if evidence.ReservedTags[ManagedByTagKey] != ManagedByTagValue ||
		evidence.ReservedTags[ScopeTagKey] != scope.Value() {
		return newObservedSlot(name, ObservedForeign, SourceIdentity{}, "", evidence.CurrentVersion, "ownership"), true
	}
	source, err := ParseSourceIdentity(evidence.ReservedTags[SourceTagKey])
	if err != nil {
		return newObservedSlot(name, ObservedInvalid, SourceIdentity{}, "", evidence.CurrentVersion, "source-tag"), true
	}
	if expectedSource != nil && source != *expectedSource {
		return newObservedSlot(name, ObservedForeign, source, "", evidence.CurrentVersion, "source"), true
	}
	if strings.TrimSpace(evidence.OwningService) != "" {
		return newObservedSlot(name, ObservedConflict, source, "", evidence.CurrentVersion, "service-owned"), true
	}
	if evidence.RotationEnabled || evidence.RotationInProgress {
		return newObservedSlot(name, ObservedConflict, source, "", evidence.CurrentVersion, "rotation"), true
	}
	if evidence.HasReplicas {
		return newObservedSlot(name, ObservedConflict, source, "", evidence.CurrentVersion, "replication"), true
	}
	if evidence.ScheduledForDeletion {
		return newObservedSlot(name, ObservedOwnedScheduled, source, "", evidence.CurrentVersion, ""), true
	}
	if !evidence.StagingValid {
		return newObservedSlot(name, ObservedConflict, source, "", evidence.CurrentVersion, "staging"), true
	}
	if evidence.CurrentVersion == "" {
		return newObservedSlot(name, ObservedOwnedWithoutCurrent, source, "", evidence.CurrentVersion, ""), true
	}

	return newObservedSlot(name, ObservedOwnedActiveUnknown, source, "", evidence.CurrentVersion, ""), false
}

// newObservedSlot constructs a slot and computes a secret-safe state fingerprint.
func newObservedSlot(
	name SecretName,
	kind ObservedKind,
	source SourceIdentity,
	secretString,
	version,
	conflictClass string,
) ObservedSlot {
	payloadDigest := sha256.Sum256([]byte(secretString))
	material := strings.Join(
		[]string{
			name.Value(),
			string(kind),
			source.Value(),
			hex.EncodeToString(payloadDigest[:]),
			version,
			conflictClass,
		},
		"\x00",
	)
	fingerprint := sha256.Sum256([]byte(material))

	return ObservedSlot{
		name:          name,
		kind:          kind,
		source:        source,
		secretString:  secretString,
		fingerprint:   hex.EncodeToString(fingerprint[:]),
		conflictClass: conflictClass,
	}
}

// Name returns the classified AWS name.
func (slot ObservedSlot) Name() SecretName {
	return slot.name
}

// Kind returns the closed observed variant.
func (slot ObservedSlot) Kind() ObservedKind {
	return slot.kind
}

// Fingerprint returns a secret-safe expected-state identity.
func (slot ObservedSlot) Fingerprint() string {
	return slot.fingerprint
}

// ConflictClass returns a non-sensitive safety classification.
func (slot ObservedSlot) ConflictClass() string {
	return slot.conflictClass
}

// Source returns the classified ownership source identity.
func (slot ObservedSlot) Source() SourceIdentity {
	return slot.source
}

// achieves reports whether an owned active string already matches the desired value.
func (slot ObservedSlot) achieves(desired DesiredSecret) bool {
	return slot.kind == ObservedOwnedActiveString && slot.source == desired.Source() &&
		desired.Value().EqualString(slot.secretString)
}

// ValidateObservedSet rejects missing desired observations and duplicate union members.
func ValidateObservedSet(desired []DesiredSecret, observed []ObservedSlot) error {
	wanted := make(map[string]struct{}, len(desired))
	for _, secret := range desired {
		name := secret.Name().Value()
		if _, exists := wanted[name]; exists {
			return errors.New("duplicate desired secret name")
		}
		wanted[name] = struct{}{}
	}
	actual := make(map[string]struct{}, len(observed))
	for _, slot := range observed {
		name := slot.Name().Value()
		if _, exists := actual[name]; exists {
			return errors.New("duplicate observed secret name")
		}
		actual[name] = struct{}{}
	}
	for name := range wanted {
		if _, exists := actual[name]; !exists {
			return errors.New("direct observations do not match desired names")
		}
	}

	return nil
}
