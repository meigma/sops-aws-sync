package domain

import (
	"errors"
	"sort"
)

const (
	operationPhaseRestore = iota + 1
	operationPhaseCreate
	operationPhaseUpdate
	operationPhaseScheduleDelete
	operationPhaseUnchanged
	operationPhaseUnknown
)

// DecisionKind identifies one pure reconciliation decision.
type DecisionKind string

const (
	// DecisionCreate creates an absent desired secret.
	DecisionCreate DecisionKind = "create"
	// DecisionUpdate writes a new current version to an owned desired secret.
	DecisionUpdate DecisionKind = "update"
	// DecisionRestore restores an owned desired secret scheduled for deletion.
	DecisionRestore DecisionKind = "restore"
	// DecisionScheduleDeletion schedules an absent owned secret for recovery-window deletion.
	DecisionScheduleDeletion DecisionKind = "schedule-deletion"
	// DecisionUnchanged records an already converged desired secret.
	DecisionUnchanged DecisionKind = "unchanged"
)

// Operation is one executable decision with a typed expected state.
type Operation struct {
	kind                DecisionKind
	name                SecretName
	source              SourceIdentity
	desired             DesiredSecret
	hasDesired          bool
	expectedFingerprint string
}

// Kind returns the operation decision.
func (operation Operation) Kind() DecisionKind {
	return operation.kind
}

// Desired returns the immutable desired secret.
func (operation Operation) Desired() DesiredSecret {
	return operation.desired
}

// HasDesired reports whether the operation targets a committed desired secret.
func (operation Operation) HasDesired() bool {
	return operation.hasDesired
}

// Name returns the operation's AWS secret name.
func (operation Operation) Name() SecretName {
	return operation.name
}

// Source returns the operation's exact source ownership identity.
func (operation Operation) Source() SourceIdentity {
	return operation.source
}

// ExpectedFingerprint returns the secret-safe precondition identity.
func (operation Operation) ExpectedFingerprint() string {
	return operation.expectedFingerprint
}

// Conflict is one non-sensitive planning conflict.
type Conflict struct {
	class string
}

// Class returns the stable non-sensitive conflict classification.
func (conflict Conflict) Class() string {
	return conflict.class
}

// Plan is a deterministic, secret-safe reconciliation result.
type Plan struct {
	operations []Operation
	unchanged  int
	conflicts  []Conflict
}

// BuildPlan classifies the union of desired names and discovered scope members.
func BuildPlan(desired []DesiredSecret, observed []ObservedSlot) (Plan, error) {
	if err := ValidateObservedSet(desired, observed); err != nil {
		return Plan{}, err
	}
	observedByName := make(map[string]ObservedSlot, len(observed))
	for _, slot := range observed {
		observedByName[slot.Name().Value()] = slot
	}
	sorted := append([]DesiredSecret(nil), desired...)
	sort.Slice(sorted, func(left, right int) bool {
		return sorted[left].Name().Value() < sorted[right].Name().Value()
	})

	plan := Plan{}
	desiredNames := make(map[string]struct{}, len(sorted))
	for _, secret := range sorted {
		desiredNames[secret.Name().Value()] = struct{}{}
		slot := observedByName[secret.Name().Value()]
		switch slot.Kind() {
		case ObservedMissing:
			plan.operations = append(plan.operations, newOperation(DecisionCreate, secret, slot))
		case ObservedOwnedActiveString:
			if slot.achieves(secret) {
				plan.unchanged++
			} else {
				plan.operations = append(plan.operations, newOperation(DecisionUpdate, secret, slot))
			}
		case ObservedOwnedActiveBinary, ObservedOwnedActiveUnknown, ObservedOwnedWithoutCurrent:
			plan.operations = append(plan.operations, newOperation(DecisionUpdate, secret, slot))
		case ObservedOwnedScheduled:
			plan.operations = append(plan.operations, newOperation(DecisionRestore, secret, slot))
		case ObservedForeign, ObservedConflict, ObservedInvalid:
			plan.conflicts = append(plan.conflicts, Conflict{class: slot.ConflictClass()})
		}
	}
	for _, slot := range observed {
		if _, desiredName := desiredNames[slot.Name().Value()]; desiredName {
			continue
		}
		switch slot.Kind() {
		case ObservedOwnedActiveString, ObservedOwnedActiveUnknown,
			ObservedOwnedActiveBinary, ObservedOwnedWithoutCurrent:
			plan.operations = append(plan.operations, newDeletionOperation(slot))
		case ObservedOwnedScheduled:
			plan.unchanged++
		case ObservedForeign, ObservedConflict, ObservedInvalid:
			plan.conflicts = append(plan.conflicts, Conflict{class: slot.ConflictClass()})
		case ObservedMissing:
		}
	}
	sort.SliceStable(plan.operations, func(left, right int) bool {
		leftPhase := operationPhase(plan.operations[left].Kind())
		rightPhase := operationPhase(plan.operations[right].Kind())
		if leftPhase != rightPhase {
			return leftPhase < rightPhase
		}
		return plan.operations[left].Name().Value() < plan.operations[right].Name().Value()
	})
	sort.Slice(plan.conflicts, func(left, right int) bool {
		return plan.conflicts[left].Class() < plan.conflicts[right].Class()
	})

	return plan, nil
}

// newOperation constructs an operation from a classified expected state.
func newOperation(kind DecisionKind, desired DesiredSecret, expected ObservedSlot) Operation {
	return Operation{
		kind: kind, name: desired.Name(), source: desired.Source(), desired: desired, hasDesired: true,
		expectedFingerprint: expected.Fingerprint(),
	}
}

// newDeletionOperation constructs one absent-source lifecycle operation.
func newDeletionOperation(expected ObservedSlot) Operation {
	return Operation{
		kind: DecisionScheduleDeletion, name: expected.Name(), source: expected.Source(),
		expectedFingerprint: expected.Fingerprint(),
	}
}

// operationPhase returns the design-defined deterministic phase order.
func operationPhase(kind DecisionKind) int {
	switch kind {
	case DecisionRestore:
		return operationPhaseRestore
	case DecisionCreate:
		return operationPhaseCreate
	case DecisionUpdate:
		return operationPhaseUpdate
	case DecisionScheduleDeletion:
		return operationPhaseScheduleDelete
	case DecisionUnchanged:
		return operationPhaseUnchanged
	}

	return operationPhaseUnknown
}

// Operations returns a defensive copy of executable operations.
func (plan Plan) Operations() []Operation {
	return append([]Operation(nil), plan.operations...)
}

// Conflicts returns a defensive copy of planning conflicts.
func (plan Plan) Conflicts() []Conflict {
	return append([]Conflict(nil), plan.conflicts...)
}

// UnchangedCount returns the number of converged desired names.
func (plan Plan) UnchangedCount() int {
	return plan.unchanged
}

// Converged reports whether no executable operation or conflict remains.
func (plan Plan) Converged() bool {
	return len(plan.operations) == 0 && len(plan.conflicts) == 0
}

// Counts returns a secret-free decision summary.
func (plan Plan) Counts() Counts {
	counts := Counts{Unchanged: plan.unchanged, Conflicts: len(plan.conflicts)}
	for _, operation := range plan.operations {
		switch operation.Kind() {
		case DecisionCreate:
			counts.Create++
		case DecisionUpdate:
			counts.Update++
		case DecisionRestore:
			counts.Restore++
		case DecisionScheduleDeletion:
			counts.ScheduleDelete++
		case DecisionUnchanged:
			counts.Unchanged++
		}
	}

	return counts
}

// Counts contains non-sensitive decision totals.
type Counts struct {
	// Create is the number of create operations.
	Create int `json:"create"`
	// Update is the number of update operations.
	Update int `json:"update"`
	// Restore is the number of restore operations.
	Restore int `json:"restore"`
	// ScheduleDelete is the number of scheduled-deletion operations.
	ScheduleDelete int `json:"schedule_delete"`
	// Unchanged is the number of no-op decisions.
	Unchanged int `json:"unchanged"`
	// Conflicts is the number of conflicts.
	Conflicts int `json:"conflicts"`
}

// ValidatePhaseTwo rejects lifecycle operations deferred to Phase 3.
func (plan Plan) ValidatePhaseTwo() error {
	if len(plan.conflicts) > 0 {
		return errors.New("plan contains a safety conflict")
	}
	for _, operation := range plan.operations {
		if operation.Kind() == DecisionRestore || operation.Kind() == DecisionScheduleDeletion {
			return errors.New("lifecycle operations are deferred until Phase 3")
		}
	}

	return nil
}

// TransitionOutcome identifies the pure result of a precondition check.
type TransitionOutcome string

const (
	// TransitionApply permits the planned write against the expected state.
	TransitionApply TransitionOutcome = "apply"
	// TransitionSucceeded proves the desired write already took effect.
	TransitionSucceeded TransitionOutcome = "succeeded"
	// TransitionRetrySameToken permits one ambiguous logical write retry.
	//nolint:gosec // G101: this domain outcome label is not credential material.
	TransitionRetrySameToken TransitionOutcome = "retry-same-token"
	// TransitionReplan reports harmless drift that changed the needed operation.
	TransitionReplan TransitionOutcome = "replan"
	// TransitionConflict reports a changed ownership or safety constraint.
	TransitionConflict TransitionOutcome = "conflict"
	// TransitionInconclusive reports an ambiguous lifecycle write that cannot be safely retried.
	TransitionInconclusive TransitionOutcome = "inconclusive"
)

// CheckPrecondition evaluates an operation immediately before mutation.
func CheckPrecondition(operation Operation, current ObservedSlot) TransitionOutcome {
	if operationAchieved(operation, current) {
		return TransitionSucceeded
	}
	if isConflictKind(current.Kind()) {
		return TransitionConflict
	}
	if current.Fingerprint() == operation.ExpectedFingerprint() {
		return TransitionApply
	}

	return TransitionReplan
}

// ResolveAmbiguous evaluates evidence after an ambiguous mutation result.
func ResolveAmbiguous(operation Operation, current ObservedSlot) TransitionOutcome {
	if operationAchieved(operation, current) {
		return TransitionSucceeded
	}
	if isConflictKind(current.Kind()) {
		return TransitionConflict
	}
	if current.Fingerprint() == operation.ExpectedFingerprint() &&
		(operation.Kind() == DecisionCreate || operation.Kind() == DecisionUpdate) {
		return TransitionRetrySameToken
	}
	if current.Fingerprint() == operation.ExpectedFingerprint() {
		return TransitionInconclusive
	}

	return TransitionReplan
}

// operationAchieved reports whether current evidence proves one operation's postcondition.
func operationAchieved(operation Operation, current ObservedSlot) bool {
	switch operation.Kind() {
	case DecisionCreate, DecisionUpdate:
		return operation.hasDesired && current.achieves(operation.desired)
	case DecisionRestore:
		return current.Source() == operation.Source() &&
			(current.Kind() == ObservedOwnedActiveString || current.Kind() == ObservedOwnedActiveUnknown ||
				current.Kind() == ObservedOwnedActiveBinary || current.Kind() == ObservedOwnedWithoutCurrent)
	case DecisionScheduleDeletion:
		return current.Source() == operation.Source() && current.Kind() == ObservedOwnedScheduled
	case DecisionUnchanged:
		return true
	}

	return false
}

// isConflictKind reports whether an observed variant blocks mutation.
func isConflictKind(kind ObservedKind) bool {
	return kind == ObservedForeign || kind == ObservedConflict || kind == ObservedInvalid
}
