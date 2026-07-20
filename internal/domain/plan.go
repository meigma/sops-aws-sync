package domain

import (
	"errors"
	"sort"
)

const (
	operationPhaseRestore = iota + 1
	operationPhaseCreate
	operationPhaseUpdate
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
	// DecisionUnchanged records an already converged desired secret.
	DecisionUnchanged DecisionKind = "unchanged"
)

// Operation is one executable decision with a typed expected state.
type Operation struct {
	kind                DecisionKind
	desired             DesiredSecret
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

// BuildPlan classifies desired names against their direct observed states.
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
	for _, secret := range sorted {
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
		case ObservedOwnedActiveBinary, ObservedOwnedWithoutCurrent:
			plan.operations = append(plan.operations, newOperation(DecisionUpdate, secret, slot))
		case ObservedOwnedScheduled:
			plan.operations = append(plan.operations, newOperation(DecisionRestore, secret, slot))
		case ObservedForeign, ObservedConflict, ObservedInvalid:
			plan.conflicts = append(plan.conflicts, Conflict{class: slot.ConflictClass()})
		}
	}
	sort.SliceStable(plan.operations, func(left, right int) bool {
		leftPhase := operationPhase(plan.operations[left].Kind())
		rightPhase := operationPhase(plan.operations[right].Kind())
		if leftPhase != rightPhase {
			return leftPhase < rightPhase
		}
		return plan.operations[left].Desired().Name().Value() < plan.operations[right].Desired().Name().Value()
	})

	return plan, nil
}

// newOperation constructs an operation from a classified expected state.
func newOperation(kind DecisionKind, desired DesiredSecret, expected ObservedSlot) Operation {
	return Operation{kind: kind, desired: desired, expectedFingerprint: expected.Fingerprint()}
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
		if operation.Kind() == DecisionRestore {
			return errors.New("restore is deferred until Phase 3")
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
)

// CheckPrecondition evaluates an operation immediately before mutation.
func CheckPrecondition(operation Operation, current ObservedSlot) TransitionOutcome {
	if current.achieves(operation.Desired()) {
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
	if current.achieves(operation.Desired()) {
		return TransitionSucceeded
	}
	if isConflictKind(current.Kind()) {
		return TransitionConflict
	}
	if current.Fingerprint() == operation.ExpectedFingerprint() {
		return TransitionRetrySameToken
	}

	return TransitionReplan
}

// isConflictKind reports whether an observed variant blocks mutation.
func isConflictKind(kind ObservedKind) bool {
	return kind == ObservedForeign || kind == ObservedConflict || kind == ObservedInvalid
}
