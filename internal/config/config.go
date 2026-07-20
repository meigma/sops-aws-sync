package config

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/meigma/sops-aws-sync/internal/domain"
)

const (
	defaultMaxEncryptedBytes = "8MiB"
	minimumRecoveryDays      = 7
	maximumRecoveryDays      = 30
	binaryUnit               = 1024
)

// Config contains fully validated CLI runtime settings.
type Config struct {
	// Repository is the local Git repository path.
	Repository string
	// Revision is the commit-ish resolved by go-git.
	Revision string
	// RepositoryID is the stable ownership identity.
	RepositoryID string
	// SourceRoot is the normalized repository-relative desired-state root.
	SourceRoot string
	// SecretPrefix is the normalized AWS name and ownership prefix.
	SecretPrefix string
	// MaxEncryptedBytes is the per-document encrypted input limit.
	MaxEncryptedBytes int
	// AllowEmpty authorizes newly scheduled deletions from an empty desired set.
	AllowEmpty bool
	// AWSRegion optionally overrides the standard AWS Region chain.
	AWSRegion string
	// AWSProfile optionally selects a shared AWS profile.
	AWSProfile string
	// RecoveryWindowDays configures the scheduled deletion recovery period.
	RecoveryWindowDays int32
	// RunTimeout bounds the whole command.
	RunTimeout time.Duration
	// OperationTimeout bounds each AWS call.
	OperationTimeout time.Duration
	// VerificationTimeout bounds direct stabilization checks.
	VerificationTimeout time.Duration
	// AWSMaxAttempts optionally overrides bounded SDK attempts.
	AWSMaxAttempts int
	// AWSMaxBackoff optionally caps SDK retry backoff.
	AWSMaxBackoff time.Duration
	// LogLevel is debug, info, warn, or error.
	LogLevel string
	// LogFormat is json or text.
	LogFormat string
	// ShowResourceNames explicitly permits names and paths in output.
	ShowResourceNames bool
	// ReportFile optionally receives the non-sensitive machine report.
	ReportFile string
}

// rawConfig mirrors the public configuration keys before validation.
type rawConfig struct {
	// Repository is the unvalidated repository path.
	Repository string `mapstructure:"repository"`
	// Revision is the unvalidated commit-ish.
	Revision string `mapstructure:"revision"`
	// RepositoryID is the unvalidated ownership identity.
	RepositoryID string `mapstructure:"repository-id"`
	// SourceRoot is the unvalidated committed source root.
	SourceRoot string `mapstructure:"source-root"`
	// SecretPrefix is the unvalidated AWS name prefix.
	SecretPrefix string `mapstructure:"secret-prefix"`
	// MaxEncryptedBytes is the unparsed byte-size limit.
	MaxEncryptedBytes string `mapstructure:"max-encrypted-bytes"`
	// AllowEmpty is the unvalidated empty-state authorization.
	AllowEmpty bool `mapstructure:"allow-empty"`
	// AWSRegion is the optional Region override.
	AWSRegion string `mapstructure:"aws-region"`
	// AWSProfile is the optional shared profile override.
	AWSProfile string `mapstructure:"aws-profile"`
	// RecoveryWindowDays is the unvalidated recovery period.
	RecoveryWindowDays int `mapstructure:"recovery-window-days"`
	// RunTimeout is the unparsed whole-run timeout.
	RunTimeout string `mapstructure:"run-timeout"`
	// OperationTimeout is the unparsed per-call timeout.
	OperationTimeout string `mapstructure:"operation-timeout"`
	// VerificationTimeout is the unparsed stabilization timeout.
	VerificationTimeout string `mapstructure:"verification-timeout"`
	// AWSMaxAttempts is the unvalidated SDK attempts override.
	AWSMaxAttempts int `mapstructure:"aws-max-attempts"`
	// AWSMaxBackoff is the unparsed SDK backoff override.
	AWSMaxBackoff string `mapstructure:"aws-max-backoff"`
	// LogLevel is the unvalidated minimum level.
	LogLevel string `mapstructure:"log-level"`
	// LogFormat is the unvalidated output format.
	LogFormat string `mapstructure:"log-format"`
	// ShowResourceNames is the explicit identifier disclosure control.
	ShowResourceNames bool `mapstructure:"show-resource-names"`
	// ReportFile is the optional report destination.
	ReportFile string `mapstructure:"report-file"`
}

// SetDefaults installs the complete stable CLI defaults on one Viper instance.
func SetDefaults(vp *viper.Viper) {
	vp.SetDefault("repository", ".")
	vp.SetDefault("revision", "HEAD")
	vp.SetDefault("source-root", "secrets")
	vp.SetDefault("max-encrypted-bytes", defaultMaxEncryptedBytes)
	vp.SetDefault("allow-empty", false)
	vp.SetDefault("recovery-window-days", maximumRecoveryDays)
	vp.SetDefault("run-timeout", "10m")
	vp.SetDefault("operation-timeout", "30s")
	vp.SetDefault("verification-timeout", "2m")
	vp.SetDefault("aws-max-attempts", 0)
	vp.SetDefault("aws-max-backoff", "0s")
	vp.SetDefault("log-level", "info")
	vp.SetDefault("log-format", "json")
	vp.SetDefault("show-resource-names", false)
}

// BindEnvironment binds every supported key to its explicit environment name.
func BindEnvironment(vp *viper.Viper) error {
	keys := []string{
		"repository", "revision", "repository-id", "source-root", "secret-prefix",
		"max-encrypted-bytes", "allow-empty", "aws-region", "aws-profile",
		"recovery-window-days", "run-timeout", "operation-timeout", "verification-timeout",
		"aws-max-attempts", "aws-max-backoff", "log-level", "log-format",
		"show-resource-names", "report-file",
	}
	for _, key := range keys {
		environment := "SOPS_AWS_SYNC_" + strings.NewReplacer("-", "_", ".", "_").Replace(strings.ToUpper(key))
		if err := vp.BindEnv(key, environment); err != nil {
			return fmt.Errorf("bind environment key %s: %w", key, err)
		}
	}

	return nil
}

// Load unmarshals exact keys and validates the complete configuration contract.
func Load(vp *viper.Viper) (Config, error) {
	var raw rawConfig
	if err := vp.UnmarshalExact(&raw); err != nil {
		return Config{}, fmt.Errorf("decode configuration: %w", err)
	}
	return raw.validate()
}

// validate converts raw values and enforces side-effect-free configuration rules.
func (raw rawConfig) validate() (Config, error) {
	if strings.TrimSpace(raw.Repository) == "" {
		return Config{}, errors.New("repository is required")
	}
	if strings.TrimSpace(raw.Revision) == "" {
		return Config{}, errors.New("revision is required")
	}
	repositoryID := strings.TrimSpace(raw.RepositoryID)
	if repositoryID == "" {
		return Config{}, errors.New("repository-id is required")
	}
	secretPrefix := strings.TrimSuffix(strings.TrimSpace(raw.SecretPrefix), "/")
	if _, err := domain.NewSecretName(secretPrefix); err != nil {
		return Config{}, fmt.Errorf("secret-prefix is invalid: %w", err)
	}
	if _, err := domain.NewScopeIdentity(repositoryID, raw.SourceRoot, secretPrefix); err != nil {
		return Config{}, fmt.Errorf("ownership scope is invalid: %w", err)
	}
	maxEncryptedBytes, err := parseByteSize(raw.MaxEncryptedBytes)
	if err != nil || maxEncryptedBytes > math.MaxInt {
		return Config{}, errors.New("max-encrypted-bytes is invalid")
	}
	runTimeout, err := parsePositiveDuration("run-timeout", raw.RunTimeout)
	if err != nil {
		return Config{}, err
	}
	operationTimeout, err := parsePositiveDuration("operation-timeout", raw.OperationTimeout)
	if err != nil {
		return Config{}, err
	}
	verificationTimeout, err := parsePositiveDuration("verification-timeout", raw.VerificationTimeout)
	if err != nil {
		return Config{}, err
	}
	maxBackoff, err := time.ParseDuration(raw.AWSMaxBackoff)
	if err != nil || maxBackoff < 0 {
		return Config{}, errors.New("aws-max-backoff is invalid")
	}
	if raw.AWSMaxAttempts < 0 {
		return Config{}, errors.New("aws-max-attempts is invalid")
	}
	if raw.RecoveryWindowDays < minimumRecoveryDays || raw.RecoveryWindowDays > maximumRecoveryDays {
		return Config{}, fmt.Errorf(
			"recovery-window-days must be between %d and %d",
			minimumRecoveryDays,
			maximumRecoveryDays,
		)
	}
	logLevel := strings.ToLower(raw.LogLevel)
	if !isOneOf(logLevel, "debug", "info", "warn", "error") {
		return Config{}, errors.New("log-level is invalid")
	}
	logFormat := strings.ToLower(raw.LogFormat)
	if !isOneOf(logFormat, "json", "text") {
		return Config{}, errors.New("log-format is invalid")
	}

	return Config{
		Repository:          raw.Repository,
		Revision:            raw.Revision,
		RepositoryID:        repositoryID,
		SourceRoot:          raw.SourceRoot,
		SecretPrefix:        secretPrefix,
		MaxEncryptedBytes:   int(maxEncryptedBytes),
		AllowEmpty:          raw.AllowEmpty,
		AWSRegion:           strings.TrimSpace(raw.AWSRegion),
		AWSProfile:          strings.TrimSpace(raw.AWSProfile),
		RecoveryWindowDays:  int32(raw.RecoveryWindowDays),
		RunTimeout:          runTimeout,
		OperationTimeout:    operationTimeout,
		VerificationTimeout: verificationTimeout,
		AWSMaxAttempts:      raw.AWSMaxAttempts,
		AWSMaxBackoff:       maxBackoff,
		LogLevel:            logLevel,
		LogFormat:           logFormat,
		ShowResourceNames:   raw.ShowResourceNames,
		ReportFile:          strings.TrimSpace(raw.ReportFile),
	}, nil
}

// parsePositiveDuration parses one strictly positive duration key.
func parsePositiveDuration(key, value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s is invalid", key)
	}

	return duration, nil
}

// parseByteSize parses an integer byte count with an optional binary suffix.
func parseByteSize(value string) (int64, error) {
	trimmed := strings.TrimSpace(value)
	multipliers := []struct {
		suffix     string
		multiplier int64
	}{
		{suffix: "GiB", multiplier: binaryUnit * binaryUnit * binaryUnit},
		{suffix: "MiB", multiplier: binaryUnit * binaryUnit},
		{suffix: "KiB", multiplier: binaryUnit},
		{suffix: "B", multiplier: 1},
	}
	multiplier := int64(1)
	for _, candidate := range multipliers {
		if withoutSuffix, found := strings.CutSuffix(trimmed, candidate.suffix); found {
			trimmed = withoutSuffix
			multiplier = candidate.multiplier
			break
		}
	}
	base, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || base <= 0 || base > math.MaxInt64/multiplier {
		return 0, errors.New("byte size is invalid")
	}

	return base * multiplier, nil
}

// isOneOf reports whether value matches one permitted literal.
func isOneOf(value string, allowed ...string) bool {
	return slices.Contains(allowed, value)
}
