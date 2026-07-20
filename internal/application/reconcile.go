package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
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

// SecretsManager is the direct observation and Phase 2 mutation port.
type SecretsManager interface {
	Observe(
		ctx context.Context,
		desired domain.DesiredSecret,
		scope domain.ScopeIdentity,
	) (domain.ObservedEvidence, error)
	Create(ctx context.Context, desired domain.DesiredSecret, scope domain.ScopeIdentity, token string) error
	Update(ctx context.Context, desired domain.DesiredSecret, token string) error
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
	// SourceRoot selects committed SOPS JSON documents.
	SourceRoot string
	// SecretPrefix defines the AWS naming and ownership boundary.
	SecretPrefix string
	// VerificationTimeout bounds direct post-apply stabilization.
	VerificationTimeout time.Duration
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

// Service orchestrates one direct desired-name reconciliation slice.
type Service struct {
	desired DesiredSnapshot
	secrets SecretsManager
	tokens  TokenSource
	logger  *slog.Logger
	version string
}

// NewService constructs the Phase 2 application service.
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

// Plan builds and reports the pure direct desired-name plan without mutation.
func (service *Service) Plan(ctx context.Context, input ReconcileInput) (Report, error) {
	started := time.Now()
	plan, _, _, revision, err := service.buildPlan(ctx, input)
	report := service.newReport(plan, revision, started)
	if err != nil {
		classified := service.classifyPreflightError(ctx, err)
		report.Status = preflightStatus(classified)
		return report, classified
	}
	if len(plan.Conflicts()) > 0 {
		report.Status = string(OutcomeConflict)
		return report, NewOutcomeError(OutcomeConflict)
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

// Sync applies supported operations and requires a converged direct re-plan.
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
	if validationErr := plan.ValidatePhaseTwo(); validationErr != nil {
		return fail(NewOutcomeError(OutcomeConflict))
	}
	for rebuilds := 0; ; rebuilds++ {
		rebuild, applyErr := service.applyPlan(ctx, scope, plan, input.ShowResourceNames)
		if applyErr != nil {
			return fail(applyErr)
		}
		if !rebuild {
			break
		}
		if rebuilds >= maximumPlanRebuilds {
			return fail(NewOutcomeError(OutcomeVerification))
		}
		rebuiltPlan, rebuildErr := service.rebuildPlan(ctx, desired, scope)
		if rebuildErr != nil {
			return fail(rebuildErr)
		}
		plan = rebuiltPlan
		report.Counts = plan.Counts()
		if err := plan.ValidatePhaseTwo(); err != nil {
			return fail(NewOutcomeError(OutcomeConflict))
		}
	}
	verified, verificationErr := service.verify(ctx, input, desired, scope)
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

// applyPlan applies one phase-ordered plan or asks the caller to rebuild all decisions.
func (service *Service) applyPlan(
	ctx context.Context,
	scope domain.ScopeIdentity,
	plan domain.Plan,
	showName bool,
) (bool, error) {
	for index, operation := range plan.Operations() {
		if err := service.applyOperation(ctx, scope, operation, index, showName); err != nil {
			if errors.Is(err, errPlanChanged) {
				return true, nil
			}

			return false, err
		}
	}

	return false, nil
}

// rebuildPlan re-observes every desired name before deriving a replacement plan.
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

// buildPlan derives scope, observes every desired name, and plans.
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

// observeAll directly observes and classifies every desired name.
func (service *Service) observeAll(
	ctx context.Context,
	desired []domain.DesiredSecret,
	scope domain.ScopeIdentity,
) ([]domain.ObservedSlot, error) {
	observed := make([]domain.ObservedSlot, 0, len(desired))
	for _, secret := range desired {
		evidence, err := service.secrets.Observe(ctx, secret, scope)
		if err != nil {
			return nil, err
		}
		observed = append(observed, domain.ClassifyDirect(secret, scope, evidence))
	}

	return observed, nil
}

// applyOperation rechecks the precondition and resolves one ambiguous logical write.
func (service *Service) applyOperation(
	ctx context.Context,
	scope domain.ScopeIdentity,
	operation domain.Operation,
	index int,
	showName bool,
) error {
	current, err := service.observeOne(ctx, scope, operation.Desired())
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
	case domain.TransitionApply:
	}
	token, err := service.tokens.NewToken()
	if err != nil {
		return NewOutcomeError(OutcomeApplyFailed)
	}
	service.logOperation(ctx, "applying", operation, index, showName)
	err = service.mutate(ctx, scope, operation, token)
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

	return service.resolveAmbiguous(ctx, scope, operation, token)
}

// resolveAmbiguous re-observes one uncertain mutation before a same-token retry.
func (service *Service) resolveAmbiguous(
	ctx context.Context,
	scope domain.ScopeIdentity,
	operation domain.Operation,
	token string,
) error {
	current, observeErr := service.observeOne(ctx, scope, operation.Desired())
	if observeErr != nil {
		return service.classifyApplyError(ctx, observeErr)
	}
	switch domain.ResolveAmbiguous(operation, current) {
	case domain.TransitionSucceeded:
		return nil
	case domain.TransitionRetrySameToken:
		if retryErr := service.mutate(ctx, scope, operation, token); retryErr != nil {
			final, finalErr := service.observeOne(ctx, scope, operation.Desired())
			if finalErr == nil && domain.ResolveAmbiguous(operation, final) == domain.TransitionSucceeded {
				return nil
			}
			return service.classifyApplyError(ctx, retryErr)
		}
		return nil
	case domain.TransitionConflict:
		return NewOutcomeError(OutcomeConflict)
	case domain.TransitionReplan, domain.TransitionApply:
		return NewOutcomeError(OutcomeApplyFailed)
	}

	return NewOutcomeError(OutcomeApplyFailed)
}

// mutate dispatches only the create and update operations supported in Phase 2.
func (service *Service) mutate(
	ctx context.Context,
	scope domain.ScopeIdentity,
	operation domain.Operation,
	token string,
) error {
	switch operation.Kind() {
	case domain.DecisionCreate:
		return service.secrets.Create(ctx, operation.Desired(), scope, token)
	case domain.DecisionUpdate:
		return service.secrets.Update(ctx, operation.Desired(), token)
	case domain.DecisionRestore, domain.DecisionUnchanged:
		return errors.New("unsupported Phase 2 mutation")
	}

	return errors.New("unknown mutation")
}

// observeOne directly classifies one desired name.
func (service *Service) observeOne(
	ctx context.Context,
	scope domain.ScopeIdentity,
	desired domain.DesiredSecret,
) (domain.ObservedSlot, error) {
	evidence, err := service.secrets.Observe(ctx, desired, scope)
	if err != nil {
		return domain.ObservedSlot{}, err
	}

	return domain.ClassifyDirect(desired, scope, evidence), nil
}

// verify retries direct re-observation until the plan converges or its deadline ends.
func (service *Service) verify(
	ctx context.Context,
	input ReconcileInput,
	desired []domain.DesiredSecret,
	scope domain.ScopeIdentity,
) (domain.Plan, error) {
	verificationContext, cancel := context.WithTimeout(ctx, input.VerificationTimeout)
	defer cancel()
	for {
		observed, err := service.observeAll(verificationContext, desired, scope)
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
		resource = operation.Desired().Name().Value()
	}
	service.logger.InfoContext(ctx, "reconciliation operation", "phase", "apply", "operation", operation.Kind(),
		"status", status, "resource", resource)
}
