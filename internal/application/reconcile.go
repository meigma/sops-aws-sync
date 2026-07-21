package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sort"
	"time"

	"github.com/meigma/sops-aws-sync/internal/domain"
)

const (
	// ReportSchemaVersion identifies the stable machine report contract.
	ReportSchemaVersion      = "sops-aws-sync/report/v1"
	statusConverged          = "converged"
	statusVerificationFailed = "verification-failed"
	verificationNotRun       = "not-run"
	verificationFailed       = "failed"
	verificationPollInterval = 250 * time.Millisecond
	maximumPlanRebuilds      = 1
)

var errPlanChanged = errors.New("reconciliation plan changed")

// applyResult retains mutation evidence that must survive harmless plan rebuilds.
type applyResult struct {
	rebuild             bool
	rebuiltAfterRestore bool
	affected            map[string]domain.SecretName
	restored            map[string]domain.SecretName
}

// newApplyResult constructs empty per-run mutation evidence.
func newApplyResult() applyResult {
	return applyResult{
		affected: make(map[string]domain.SecretName),
		restored: make(map[string]domain.SecretName),
	}
}

// merge retains every applied target and restore across one or more plan attempts.
func (result *applyResult) merge(other applyResult) {
	result.rebuild = other.rebuild
	result.rebuiltAfterRestore = result.rebuiltAfterRestore || other.rebuiltAfterRestore
	maps.Copy(result.affected, other.affected)
	maps.Copy(result.restored, other.restored)
}

// affect retains one planned target that must be directly verified after apply.
func (result *applyResult) affect(operation domain.Operation) {
	name := operation.Name()
	result.affected[name.Value()] = name
}

// record retains one operation whose mutation outcome was proved successful.
func (result *applyResult) record(operation domain.Operation) {
	result.affect(operation)
	name := operation.Name()
	if operation.Kind() == domain.DecisionRestore {
		result.restored[name.Value()] = name
	}
}

// SecretsManager is the scope discovery, direct observation, and lifecycle mutation port.
type SecretsManager interface {
	Discover(ctx context.Context) ([]domain.DiscoveryEvidence, error)
	Observe(
		ctx context.Context,
		desired domain.DesiredSecret,
		scope domain.ScopeIdentity,
	) (domain.ObservedEvidence, error)
	ObserveManaged(ctx context.Context, name domain.SecretName) (domain.ObservedEvidence, error)
	Create(ctx context.Context, desired domain.DesiredSecret, scope domain.ScopeIdentity, token string) error
	Update(ctx context.Context, desired domain.DesiredSecret, token string) error
	Restore(ctx context.Context, name domain.SecretName) error
	ScheduleDeletion(ctx context.Context, name domain.SecretName, recoveryWindowDays int32) error
}

// TokenSource produces fresh logical-write idempotency tokens.
type TokenSource interface {
	NewToken() (string, error)
}

// RandomTokenSource generates cryptographically random 128-bit hexadecimal tokens.
type RandomTokenSource struct{}

// NewToken returns a fresh 32-character Secrets Manager client request token.
func (RandomTokenSource) NewToken() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", errors.New("generate logical-write token")
	}

	return hex.EncodeToString(token[:]), nil
}

// ReconcileInput configures one immutable desired snapshot and direct verification.
type ReconcileInput struct {
	// RepositoryID is the stable ownership identity.
	RepositoryID string
	// SourceRoot selects committed SOPS JSON or YAML documents.
	SourceRoot string
	// SecretPrefix defines the AWS naming and ownership boundary.
	SecretPrefix string
	// VerificationTimeout bounds direct post-apply stabilization.
	VerificationTimeout time.Duration
	// AllowEmpty authorizes newly scheduled deletions from an empty desired snapshot.
	AllowEmpty bool
	// RecoveryWindowDays is the explicit Secrets Manager recovery period.
	RecoveryWindowDays int32
	// ShowResourceNames explicitly enables names in operational logs.
	ShowResourceNames bool
}

// Report is the non-sensitive stable machine result.
type Report struct {
	// SchemaVersion identifies the report contract.
	SchemaVersion string `json:"schema_version"`
	// ToolVersion is the running CLI release.
	ToolVersion string `json:"tool_version"`
	// GitRevision is the exact desired commit.
	GitRevision string `json:"git_revision"`
	// Status is the stable command outcome.
	Status string `json:"status"`
	// Counts contains only decision totals.
	Counts domain.Counts `json:"counts"`
	// Verification is not-run, converged, failed, or inconclusive.
	Verification string `json:"verification"`
	// DurationMilliseconds is the whole use-case duration.
	DurationMilliseconds int64 `json:"duration_ms"`
}

// OutcomeKind identifies a stable CLI exit class.
type OutcomeKind string

const (
	// OutcomeInvalid identifies configuration or desired-state failure.
	OutcomeInvalid OutcomeKind = "invalid"
	// OutcomeConflict identifies an ownership or safety conflict.
	OutcomeConflict OutcomeKind = "conflict"
	// OutcomeApplyFailed identifies a failed or unknown mutation result.
	OutcomeApplyFailed OutcomeKind = "apply-failed"
	// OutcomeVerification identifies failed or inconclusive verification.
	OutcomeVerification OutcomeKind = "verification"
	// OutcomeInterrupted identifies cancellation.
	OutcomeInterrupted OutcomeKind = "interrupted"
)

// OutcomeError carries a safe stable failure class without raw infrastructure text.
type OutcomeError struct {
	kind  OutcomeKind
	cause error
}

// NewOutcomeError constructs a stable safe outcome error.
func NewOutcomeError(kind OutcomeKind) *OutcomeError {
	return &OutcomeError{kind: kind}
}

// newOutcomeCause constructs a safe outcome that retains typed internal evidence.
func newOutcomeCause(kind OutcomeKind, cause error) *OutcomeError {
	return &OutcomeError{kind: kind, cause: cause}
}

// Error returns the stable failure class.
func (failure *OutcomeError) Error() string {
	return string(failure.kind)
}

// Kind returns the stable failure classification.
func (failure *OutcomeError) Kind() OutcomeKind {
	return failure.kind
}

// Unwrap exposes typed internal evidence without changing the safe error string.
func (failure *OutcomeError) Unwrap() error {
	return failure.cause
}

// Service orchestrates the complete desired and discovered-scope reconciliation lifecycle.
type Service struct {
	desired DesiredSnapshot
	secrets SecretsManager
	tokens  TokenSource
	logger  *slog.Logger
	version string
}

// NewService constructs the V1 application service.
func NewService(
	desired DesiredSnapshot,
	secrets SecretsManager,
	tokens TokenSource,
	logger *slog.Logger,
	version string,
) (*Service, error) {
	if desired.Revision().Value() == "" || secrets == nil || tokens == nil {
		return nil, errors.New("application ports are required")
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	return &Service{
		desired: desired,
		secrets: secrets,
		tokens:  tokens,
		logger:  logger,
		version: version,
	}, nil
}

// Plan builds and reports the pure scope-wide plan without mutation.
func (service *Service) Plan(ctx context.Context, input ReconcileInput) (Report, error) {
	started := time.Now()
	plan, desired, _, revision, err := service.buildPlan(ctx, input)
	report := service.newReport(plan, revision, started)
	if err != nil {
		classified := service.classifyPreflightError(ctx, err)
		report.Status = preflightStatus(classified)
		return report, classified
	}
	if validationErr := validatePlan(desired, plan, input.AllowEmpty); validationErr != nil {
		report.Status, _ = outcomeReport(validationErr)
		return report, validationErr
	}
	if plan.Converged() {
		report.Status = statusConverged
	} else {
		report.Status = "drift"
	}
	service.logger.InfoContext(ctx, "plan complete", "phase", "plan", "status", report.Status,
		"count", len(plan.Operations()), "duration_ms", time.Since(started).Milliseconds())
	report.DurationMilliseconds = time.Since(started).Milliseconds()

	return report, nil
}

// Sync applies the complete V1 operation set and requires a converged scope-wide re-plan.
//
//nolint:nonamedreturns // The named report lets deferred duration stamping cover every exit.
func (service *Service) Sync(ctx context.Context, input ReconcileInput) (report Report, resultErr error) {
	started := time.Now()
	defer func() {
		report.DurationMilliseconds = time.Since(started).Milliseconds()
	}()
	plan, desired, scope, revision, err := service.buildPlan(ctx, input)
	report = service.newReport(plan, revision, started)
	fail := func(failure error) (Report, error) {
		report.Status, report.Verification = outcomeReport(failure)

		return report, failure
	}
	if err != nil {
		classified := service.classifyPreflightError(ctx, err)
		report.Status = preflightStatus(classified)
		return report, classified
	}
	var applied applyResult
	plan, applied, err = service.applyWithRebuilds(ctx, input, desired, scope, plan)
	report.Counts = plan.Counts()
	if err != nil {
		return fail(err)
	}
	followUpCounts, followUpApplied, err := service.applyRestoreFollowUp(
		ctx,
		input,
		desired,
		scope,
		plan,
		applied,
	)
	if err != nil {
		return fail(err)
	}
	applied.merge(followUpApplied)
	report.Counts = addExecutableCounts(report.Counts, followUpCounts)
	verified, verificationErr := service.verify(ctx, input, desired, scope, applied.affected)
	report.Verification = statusConverged
	if verificationErr != nil {
		report.Status, report.Verification = verificationErrorReport(verificationErr)
		return report, verificationErr
	}
	if !verified.Converged() {
		return fail(NewOutcomeError(OutcomeVerification))
	}
	report.Status = statusConverged
	service.logger.InfoContext(ctx, "sync complete", "phase", "verify", "status", statusConverged,
		"count", len(plan.Operations()), "duration_ms", time.Since(started).Milliseconds())

	return report, nil
}

// applyWithRebuilds applies one complete plan and permits one harmless precondition rebuild.
func (service *Service) applyWithRebuilds(
	ctx context.Context,
	input ReconcileInput,
	desired []domain.DesiredSecret,
	scope domain.ScopeIdentity,
	plan domain.Plan,
) (domain.Plan, applyResult, error) {
	applied := newApplyResult()
	if err := validatePlan(desired, plan, input.AllowEmpty); err != nil {
		return plan, applied, err
	}
	for rebuilds := 0; ; rebuilds++ {
		attempt, err := service.applyPlan(
			ctx,
			scope,
			plan,
			input.RecoveryWindowDays,
			input.ShowResourceNames,
		)
		applied.merge(attempt)
		if err != nil || !attempt.rebuild {
			return plan, applied, err
		}
		if rebuilds >= maximumPlanRebuilds {
			return plan, applied, NewOutcomeError(OutcomeVerification)
		}
		plan, err = service.rebuildPlan(ctx, desired, scope)
		if err != nil {
			return plan, applied, err
		}
		if err = validatePlan(desired, plan, input.AllowEmpty); err != nil {
			return plan, applied, err
		}
		if len(applied.restored) > 0 {
			applied.rebuiltAfterRestore = true
			return plan, applied, nil
		}
	}
}

// applyRestoreFollowUp performs at most one update-only cycle for initially restored names.
func (service *Service) applyRestoreFollowUp(
	ctx context.Context,
	input ReconcileInput,
	desired []domain.DesiredSecret,
	scope domain.ScopeIdentity,
	initial domain.Plan,
	applied applyResult,
) (domain.Counts, applyResult, error) {
	followUpApplied := newApplyResult()
	if len(applied.restored) == 0 {
		return domain.Counts{}, followUpApplied, nil
	}
	followUp := initial
	if applied.rebuiltAfterRestore {
		pending, err := validateRestoreFollowUp(followUp, applied.restored)
		if err != nil {
			return followUp.Counts(), followUpApplied, err
		}
		if !pending {
			return service.applyRestorePlan(ctx, input, scope, followUp)
		}
	}
	var err error
	followUp, err = service.waitForRestoreFollowUp(ctx, input, desired, scope, applied.restored)
	if err != nil || len(followUp.Operations()) == 0 {
		return followUp.Counts(), followUpApplied, err
	}

	return service.applyRestorePlan(ctx, input, scope, followUp)
}

// applyRestorePlan applies the single permitted restore follow-up cycle without rebuilding.
func (service *Service) applyRestorePlan(
	ctx context.Context,
	input ReconcileInput,
	scope domain.ScopeIdentity,
	followUp domain.Plan,
) (domain.Counts, applyResult, error) {
	applied, err := service.applyPlan(
		ctx,
		scope,
		followUp,
		input.RecoveryWindowDays,
		input.ShowResourceNames,
	)
	if err != nil {
		return followUp.Counts(), applied, err
	}
	if applied.rebuild {
		return followUp.Counts(), applied, NewOutcomeError(OutcomeVerification)
	}

	return followUp.Counts(), applied, nil
}

// applyPlan applies one phase-ordered plan or asks the caller to rebuild all decisions.
func (service *Service) applyPlan(
	ctx context.Context,
	scope domain.ScopeIdentity,
	plan domain.Plan,
	recoveryWindowDays int32,
	showName bool,
) (applyResult, error) {
	result := newApplyResult()
	for index, operation := range plan.Operations() {
		result.affect(operation)
		if err := service.applyOperation(
			ctx,
			scope,
			operation,
			recoveryWindowDays,
			index,
			showName,
		); err != nil {
			if errors.Is(err, errPlanChanged) {
				result.rebuild = true
				return result, nil
			}

			return result, err
		}
		result.record(operation)
	}

	return result, nil
}

// rebuildPlan re-observes the complete desired and discovered scope union.
func (service *Service) rebuildPlan(
	ctx context.Context,
	desired []domain.DesiredSecret,
	scope domain.ScopeIdentity,
) (domain.Plan, error) {
	observed, err := service.observeAll(ctx, desired, scope)
	if err != nil {
		return domain.Plan{}, service.classifyApplyError(ctx, err)
	}
	plan, err := domain.BuildPlan(desired, observed)
	if err != nil {
		return domain.Plan{}, NewOutcomeError(OutcomeApplyFailed)
	}

	return plan, nil
}

// buildPlan derives scope, observes the desired and discovered union, and plans.
func (service *Service) buildPlan(
	ctx context.Context,
	input ReconcileInput,
) (domain.Plan, []domain.DesiredSecret, domain.ScopeIdentity, domain.Revision, error) {
	desired := service.desired.copySecrets()
	revision := service.desired.Revision()
	if err := ctx.Err(); err != nil {
		return domain.Plan{}, desired, domain.ScopeIdentity{}, revision, newOutcomeCause(OutcomeInterrupted, err)
	}
	scope, err := domain.NewScopeIdentity(input.RepositoryID, input.SourceRoot, input.SecretPrefix)
	if err != nil {
		return domain.Plan{}, nil, domain.ScopeIdentity{}, revision, newOutcomeCause(OutcomeInvalid, err)
	}
	observed, err := service.observeAll(ctx, desired, scope)
	if err != nil {
		return domain.Plan{}, desired, scope, revision, newOutcomeCause(OutcomeApplyFailed, err)
	}
	plan, err := domain.BuildPlan(desired, observed)
	if err != nil {
		return domain.Plan{}, desired, scope, revision, newOutcomeCause(OutcomeInvalid, err)
	}

	return plan, desired, scope, revision, nil
}

// observeAll merges paginated exact-scope discovery with direct desired-name evidence.
func (service *Service) observeAll(
	ctx context.Context,
	desired []domain.DesiredSecret,
	scope domain.ScopeIdentity,
) ([]domain.ObservedSlot, error) {
	return service.observeAllWithAffected(ctx, desired, scope, nil)
}

// observeAllWithAffected directly observes every applied target even when discovery omits it.
func (service *Service) observeAllWithAffected(
	ctx context.Context,
	desired []domain.DesiredSecret,
	scope domain.ScopeIdentity,
	affected map[string]domain.SecretName,
) ([]domain.ObservedSlot, error) {
	discovered, err := service.secrets.Discover(ctx)
	if err != nil {
		return nil, err
	}
	desiredNames := make(map[string]struct{}, len(desired))
	observed := make([]domain.ObservedSlot, 0, len(desired)+len(discovered))
	for _, secret := range desired {
		evidence, observeErr := service.secrets.Observe(ctx, secret, scope)
		if observeErr != nil {
			return nil, observeErr
		}
		desiredNames[secret.Name().Value()] = struct{}{}
		observed = append(observed, domain.ClassifyDirect(secret, scope, evidence))
	}
	candidates := make(map[string]domain.SecretName, len(discovered))
	for _, candidate := range discovered {
		if !domain.IsScopeCandidate(scope, candidate.Evidence) {
			continue
		}
		name := candidate.Name.Value()
		if _, desiredName := desiredNames[name]; desiredName {
			continue
		}
		if _, duplicate := candidates[name]; duplicate {
			return nil, errors.New("scope discovery returned a duplicate name")
		}
		candidates[name] = candidate.Name
	}
	for name, target := range affected {
		if _, desiredName := desiredNames[name]; !desiredName {
			candidates[name] = target
		}
	}
	sortedNames := make([]string, 0, len(candidates))
	for name := range candidates {
		sortedNames = append(sortedNames, name)
	}
	sort.Strings(sortedNames)
	for _, candidateName := range sortedNames {
		name := candidates[candidateName]
		evidence, observeErr := service.secrets.ObserveManaged(ctx, name)
		if observeErr != nil {
			return nil, observeErr
		}
		observed = append(observed, domain.ClassifyManaged(name, scope, evidence))
	}

	return observed, nil
}

// applyOperation rechecks the precondition and resolves one ambiguous logical write.
func (service *Service) applyOperation(
	ctx context.Context,
	scope domain.ScopeIdentity,
	operation domain.Operation,
	recoveryWindowDays int32,
	index int,
	showName bool,
) error {
	current, err := service.observeOperation(ctx, scope, operation)
	if err != nil {
		return service.classifyApplyError(ctx, err)
	}
	switch domain.CheckPrecondition(operation, current) {
	case domain.TransitionSucceeded, domain.TransitionReplan:
		return errPlanChanged
	case domain.TransitionConflict:
		return NewOutcomeError(OutcomeConflict)
	case domain.TransitionRetrySameToken:
		return NewOutcomeError(OutcomeApplyFailed)
	case domain.TransitionInconclusive:
		return NewOutcomeError(OutcomeApplyFailed)
	case domain.TransitionApply:
	}
	token := ""
	if operation.Kind() == domain.DecisionCreate || operation.Kind() == domain.DecisionUpdate {
		token, err = service.tokens.NewToken()
		if err != nil {
			return NewOutcomeError(OutcomeApplyFailed)
		}
	}
	service.logOperation(ctx, "applying", operation, index, showName)
	err = service.mutate(ctx, scope, operation, token, recoveryWindowDays)
	if err == nil {
		return nil
	}
	var ambiguous interface {
		Ambiguous() bool
		Canceled() bool
	}
	if !errors.As(err, &ambiguous) || !ambiguous.Ambiguous() {
		return service.classifyApplyError(ctx, err)
	}

	return service.resolveAmbiguous(ctx, scope, operation, token, recoveryWindowDays)
}

// resolveAmbiguous re-observes one uncertain mutation before a same-token retry.
func (service *Service) resolveAmbiguous(
	ctx context.Context,
	scope domain.ScopeIdentity,
	operation domain.Operation,
	token string,
	recoveryWindowDays int32,
) error {
	current, observeErr := service.observeOperation(ctx, scope, operation)
	if observeErr != nil {
		return service.classifyApplyError(ctx, observeErr)
	}
	switch domain.ResolveAmbiguous(operation, current) {
	case domain.TransitionSucceeded:
		return nil
	case domain.TransitionRetrySameToken:
		if retryErr := service.mutate(ctx, scope, operation, token, recoveryWindowDays); retryErr != nil {
			final, finalErr := service.observeOperation(ctx, scope, operation)
			if finalErr == nil && domain.ResolveAmbiguous(operation, final) == domain.TransitionSucceeded {
				return nil
			}
			return service.classifyApplyError(ctx, retryErr)
		}
		return nil
	case domain.TransitionConflict:
		return NewOutcomeError(OutcomeConflict)
	case domain.TransitionReplan, domain.TransitionApply, domain.TransitionInconclusive:
		return NewOutcomeError(OutcomeApplyFailed)
	}

	return NewOutcomeError(OutcomeApplyFailed)
}

// mutate dispatches the complete V1 operation set.
func (service *Service) mutate(
	ctx context.Context,
	scope domain.ScopeIdentity,
	operation domain.Operation,
	token string,
	recoveryWindowDays int32,
) error {
	switch operation.Kind() {
	case domain.DecisionCreate:
		return service.secrets.Create(ctx, operation.Desired(), scope, token)
	case domain.DecisionUpdate:
		return service.secrets.Update(ctx, operation.Desired(), token)
	case domain.DecisionRestore:
		return service.secrets.Restore(ctx, operation.Name())
	case domain.DecisionScheduleDeletion:
		return service.secrets.ScheduleDeletion(ctx, operation.Name(), recoveryWindowDays)
	case domain.DecisionUnchanged:
		return errors.New("unchanged decisions are not executable")
	}

	return errors.New("unknown mutation")
}

// observeOperation directly classifies one desired or discovered operation target.
func (service *Service) observeOperation(
	ctx context.Context,
	scope domain.ScopeIdentity,
	operation domain.Operation,
) (domain.ObservedSlot, error) {
	if operation.HasDesired() {
		desired := operation.Desired()
		evidence, err := service.secrets.Observe(ctx, desired, scope)
		if err != nil {
			return domain.ObservedSlot{}, err
		}

		return domain.ClassifyDirect(desired, scope, evidence), nil
	}
	evidence, err := service.secrets.ObserveManaged(ctx, operation.Name())
	if err != nil {
		return domain.ObservedSlot{}, err
	}

	return domain.ClassifyManaged(operation.Name(), scope, evidence), nil
}

// emptyStateBlocked reports a newly destructive empty snapshot without explicit authorization.
func emptyStateBlocked(desired []domain.DesiredSecret, plan domain.Plan, allowEmpty bool) bool {
	return len(desired) == 0 && plan.Counts().ScheduleDelete > 0 && !allowEmpty
}

// validatePlan rejects conflicts and unauthorized destructive empty snapshots before mutation.
func validatePlan(desired []domain.DesiredSecret, plan domain.Plan, allowEmpty bool) error {
	if len(plan.Conflicts()) > 0 {
		return NewOutcomeError(OutcomeConflict)
	}
	if emptyStateBlocked(desired, plan, allowEmpty) {
		return NewOutcomeError(OutcomeInvalid)
	}

	return nil
}

// addExecutableCounts adds follow-up operations without double-counting repeated no-op observations.
func addExecutableCounts(base, additional domain.Counts) domain.Counts {
	base.Create += additional.Create
	base.Update += additional.Update
	base.Restore += additional.Restore
	base.ScheduleDelete += additional.ScheduleDelete

	return base
}

// waitForRestoreFollowUp waits until restored names expose only their one permitted update cycle.
func (service *Service) waitForRestoreFollowUp(
	ctx context.Context,
	input ReconcileInput,
	desired []domain.DesiredSecret,
	scope domain.ScopeIdentity,
	restored map[string]domain.SecretName,
) (domain.Plan, error) {
	verificationContext, cancel := context.WithTimeout(ctx, input.VerificationTimeout)
	defer cancel()
	for {
		plan, err := service.rebuildPlan(verificationContext, desired, scope)
		if err == nil {
			pending, validationErr := validateRestoreFollowUp(plan, restored)
			if validationErr != nil {
				return domain.Plan{}, validationErr
			}
			if !pending {
				return plan, nil
			}
		}
		select {
		case <-verificationContext.Done():
			return domain.Plan{}, classifyVerificationError(ctx)
		case <-time.After(verificationPollInterval):
		}
	}
}

// validateRestoreFollowUp accepts only waiting restores or updates for initially restored names.
func validateRestoreFollowUp(plan domain.Plan, restored map[string]domain.SecretName) (bool, error) {
	if len(plan.Conflicts()) > 0 {
		return false, NewOutcomeError(OutcomeConflict)
	}
	pending := false
	for _, operation := range plan.Operations() {
		if _, expected := restored[operation.Name().Value()]; !expected {
			return false, NewOutcomeError(OutcomeVerification)
		}
		switch operation.Kind() {
		case domain.DecisionRestore:
			pending = true
		case domain.DecisionUpdate:
		case domain.DecisionCreate, domain.DecisionScheduleDeletion, domain.DecisionUnchanged:
			return false, NewOutcomeError(OutcomeVerification)
		}
	}

	return pending, nil
}

// verify retries scope-wide re-observation until the plan converges or its deadline ends.
func (service *Service) verify(
	ctx context.Context,
	input ReconcileInput,
	desired []domain.DesiredSecret,
	scope domain.ScopeIdentity,
	affected map[string]domain.SecretName,
) (domain.Plan, error) {
	verificationContext, cancel := context.WithTimeout(ctx, input.VerificationTimeout)
	defer cancel()
	for {
		observed, err := service.observeAllWithAffected(verificationContext, desired, scope, affected)
		if err == nil {
			plan, planErr := domain.BuildPlan(desired, observed)
			if planErr != nil {
				return domain.Plan{}, NewOutcomeError(OutcomeVerification)
			}
			if plan.Converged() || len(plan.Conflicts()) > 0 {
				return plan, nil
			}
		}
		select {
		case <-verificationContext.Done():
			return domain.Plan{}, classifyVerificationError(ctx)
		case <-time.After(verificationPollInterval):
		}
	}
}

// newReport constructs the stable report skeleton.
func (service *Service) newReport(
	plan domain.Plan,
	revision domain.Revision,
	started time.Time,
) Report {
	return Report{
		SchemaVersion:        ReportSchemaVersion,
		ToolVersion:          service.version,
		GitRevision:          revision.Value(),
		Status:               string(OutcomeInvalid),
		Counts:               plan.Counts(),
		Verification:         verificationNotRun,
		DurationMilliseconds: time.Since(started).Milliseconds(),
	}
}

// classifyPreflightError maps cancellation separately from invalid or unavailable input.
func (service *Service) classifyPreflightError(ctx context.Context, err error) error {
	service.logSafeAWSError(ctx, err)
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return NewOutcomeError(OutcomeInterrupted)
	}
	var outcome *OutcomeError
	if errors.As(err, &outcome) {
		return outcome
	}

	return NewOutcomeError(OutcomeInvalid)
}

// preflightStatus maps one safe preflight failure to report status metadata.
func preflightStatus(err error) string {
	var outcome *OutcomeError
	if !errors.As(err, &outcome) {
		return string(OutcomeInvalid)
	}
	switch outcome.Kind() {
	case OutcomeApplyFailed:
		return "observation-failed"
	case OutcomeInterrupted:
		return "interrupted"
	case OutcomeConflict:
		return string(OutcomeConflict)
	case OutcomeInvalid, OutcomeVerification:
		return string(OutcomeInvalid)
	}

	return string(OutcomeInvalid)
}

// outcomeReport maps typed failures to consistent machine status fields.
func outcomeReport(err error) (string, string) {
	var outcome *OutcomeError
	if !errors.As(err, &outcome) {
		return string(OutcomeApplyFailed), verificationNotRun
	}
	switch outcome.Kind() {
	case OutcomeConflict:
		return string(OutcomeConflict), verificationNotRun
	case OutcomeInterrupted:
		return string(OutcomeInterrupted), verificationNotRun
	case OutcomeVerification:
		return statusVerificationFailed, verificationFailed
	case OutcomeInvalid:
		return string(OutcomeInvalid), verificationNotRun
	case OutcomeApplyFailed:
		return string(OutcomeApplyFailed), verificationNotRun
	}

	return string(OutcomeApplyFailed), verificationNotRun
}

// verificationErrorReport distinguishes stabilization timeout from parent interruption.
func verificationErrorReport(err error) (string, string) {
	var outcome *OutcomeError
	if errors.As(err, &outcome) && outcome.Kind() == OutcomeVerification {
		return "verification-inconclusive", "inconclusive"
	}

	return outcomeReport(err)
}

// classifyApplyError maps canceled and unknown writes to stable result classes.
func (service *Service) classifyApplyError(ctx context.Context, err error) error {
	service.logSafeAWSError(ctx, err)
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return NewOutcomeError(OutcomeInterrupted)
	}

	return NewOutcomeError(OutcomeApplyFailed)
}

// logSafeAWSError emits only normalized AWS failure metadata when available.
func (service *Service) logSafeAWSError(ctx context.Context, err error) {
	var safe interface {
		Operation() string
		Code() string
		Fault() string
		RequestID() string
	}
	if !errors.As(err, &safe) {
		return
	}
	service.logger.ErrorContext(ctx, "AWS operation failed",
		"phase", "aws", "operation", safe.Operation(), "status", "failed",
		"aws_error_code", safe.Code(), "fault", safe.Fault(), "request_id", safe.RequestID())
}

// classifyVerificationError preserves parent cancellation versus stabilization timeout.
func classifyVerificationError(parent context.Context) error {
	if parent.Err() != nil {
		return NewOutcomeError(OutcomeInterrupted)
	}

	return NewOutcomeError(OutcomeVerification)
}

// logOperation emits a safe ordinal unless names were explicitly enabled.
func (service *Service) logOperation(
	ctx context.Context,
	status string,
	operation domain.Operation,
	index int,
	showName bool,
) {
	resource := fmt.Sprintf("resource-%04d", index+1)
	if showName {
		resource = operation.Name().Value()
	}
	service.logger.InfoContext(ctx, "reconciliation operation", "phase", "apply", "operation", operation.Kind(),
		"status", status, "resource", resource)
}
