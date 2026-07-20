package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
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

// ObservedKind identifies a closed directly observed state.
type ObservedKind string

const (
	// ObservedMissing is an absent desired name.
	ObservedMissing ObservedKind = "missing"
	// ObservedForeign is a same-name secret outside the exact ownership boundary.
	ObservedForeign ObservedKind = "foreign"
	// ObservedOwnedActiveString is an owned active secret with a current string value.
	ObservedOwnedActiveString ObservedKind = "owned-active-string"
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
	name := desired.Name()
	if !evidence.Exists {
		return newObservedSlot(name, ObservedMissing, SourceIdentity{}, "", "", "")
	}
	if evidence.ReservedTags == nil {
		return newObservedSlot(name, ObservedForeign, SourceIdentity{}, "", evidence.CurrentVersion, "ownership")
	}
	if evidence.ReservedTags[ManagedByTagKey] != ManagedByTagValue ||
		evidence.ReservedTags[ScopeTagKey] != scope.Value() {
		return newObservedSlot(name, ObservedForeign, SourceIdentity{}, "", evidence.CurrentVersion, "ownership")
	}
	source, err := ParseSourceIdentity(evidence.ReservedTags[SourceTagKey])
	if err != nil {
		return newObservedSlot(name, ObservedInvalid, SourceIdentity{}, "", evidence.CurrentVersion, "source-tag")
	}
	if source != desired.Source() {
		return newObservedSlot(name, ObservedForeign, source, "", evidence.CurrentVersion, "source")
	}
	if strings.TrimSpace(evidence.OwningService) != "" {
		return newObservedSlot(name, ObservedConflict, source, "", evidence.CurrentVersion, "service-owned")
	}
	if evidence.RotationEnabled || evidence.RotationInProgress {
		return newObservedSlot(name, ObservedConflict, source, "", evidence.CurrentVersion, "rotation")
	}
	if evidence.HasReplicas {
		return newObservedSlot(name, ObservedConflict, source, "", evidence.CurrentVersion, "replication")
	}
	if evidence.ScheduledForDeletion {
		return newObservedSlot(name, ObservedOwnedScheduled, source, "", evidence.CurrentVersion, "")
	}
	if !evidence.StagingValid {
		return newObservedSlot(name, ObservedConflict, source, "", evidence.CurrentVersion, "staging")
	}
	if evidence.SecretBinary {
		if evidence.SecretString != nil {
			return newObservedSlot(name, ObservedInvalid, source, "", evidence.CurrentVersion, "payload")
		}
		return newObservedSlot(name, ObservedOwnedActiveBinary, source, "", evidence.CurrentVersion, "")
	}
	if evidence.SecretString == nil {
		return newObservedSlot(name, ObservedOwnedWithoutCurrent, source, "", evidence.CurrentVersion, "")
	}

	return newObservedSlot(name, ObservedOwnedActiveString, source, *evidence.SecretString, evidence.CurrentVersion, "")
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

// achieves reports whether an owned active string already matches the desired value.
func (slot ObservedSlot) achieves(desired DesiredSecret) bool {
	return slot.kind == ObservedOwnedActiveString && slot.source == desired.Source() &&
		desired.Value().EqualString(slot.secretString)
}

// ValidateObservedSet rejects missing or duplicate direct observations.
func ValidateObservedSet(desired []DesiredSecret, observed []ObservedSlot) error {
	if len(desired) != len(observed) {
		return errors.New("direct observation count does not match desired count")
	}
	wanted := make([]string, 0, len(desired))
	actual := make([]string, 0, len(observed))
	for _, secret := range desired {
		wanted = append(wanted, secret.Name().Value())
	}
	for _, slot := range observed {
		actual = append(actual, slot.Name().Value())
	}
	sort.Strings(wanted)
	sort.Strings(actual)
	for index := range wanted {
		if wanted[index] != actual[index] {
			return errors.New("direct observations do not match desired names")
		}
		if index > 0 && wanted[index-1] == wanted[index] {
			return errors.New("duplicate desired secret name")
		}
	}

	return nil
}
