// Package config loads and saves Zev settings in a local YAML file.
package config

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/ra1phdd/zev/internal/core/security"
)

const currentConfigVersion = 1

const (
	// SecurityModeAuto applies the configured security policy automatically.
	SecurityModeAuto = "auto"
	// SecurityModeNormal is kept as a source-compatibility alias for SecurityModeAuto.
	SecurityModeNormal = SecurityModeAuto
	// SecurityModeBypass explicitly disables permission decisions for the invocation.
	SecurityModeBypass  = "bypass"
	securityDecisionAsk = "ask"
	securityStageLocal  = "local"
)

// SecurityConfig contains the user-selected global security mode and the
// version-one policy configuration. Rules tighten built-in safeguards and
// cannot create a global allow.
type SecurityConfig struct {
	Version           int                      `yaml:"version,omitempty"`
	Mode              string                   `yaml:"mode"`
	DefaultDecision   string                   `yaml:"default_decision,omitempty"` //nolint:tagliatelle // YAML wire format uses snake_case.
	Probe             SecurityStageConfig      `yaml:"probe,omitempty"`
	Classifier        SecurityStageConfig      `yaml:"classifier,omitempty"`
	Limits            SecurityLimits           `yaml:"limits,omitempty"`
	Rules             SecurityRules            `yaml:"rules,omitempty"`
	DangerousCommands SecurityCommandOverrides `yaml:"dangerous_commands,omitempty"` //nolint:tagliatelle // YAML wire format uses snake_case.
	DeniedCommands    []string                 `yaml:"denied_commands,omitempty"`    //nolint:tagliatelle // YAML wire format uses snake_case.
	WebFetchEnabled   bool                     `yaml:"web_fetch_enabled"`            //nolint:tagliatelle // YAML wire format uses snake_case.
}

// SecurityCommandOverrides contains exact command strings added to or removed
// from the built-in dangerous-command classifications.
type SecurityCommandOverrides struct {
	Add    []string `yaml:"add,omitempty"`
	Remove []string `yaml:"remove,omitempty"`
}

// SecurityStageConfig configures one local analysis stage.
type SecurityStageConfig struct {
	Mode         string `yaml:"mode,omitempty"`
	Provider     string `yaml:"provider,omitempty"`
	LocalActions string `yaml:"local_actions,omitempty"` //nolint:tagliatelle // YAML wire format uses snake_case.
	OnFailure    string `yaml:"on_failure,omitempty"`    //nolint:tagliatelle // YAML wire format uses snake_case.
}

// SecurityLimits bounds security review work.
type SecurityLimits struct {
	ReviewTimeout      string `yaml:"review_timeout,omitempty"`       //nolint:tagliatelle // YAML wire format uses snake_case.
	TotalReviewTimeout string `yaml:"total_review_timeout,omitempty"` //nolint:tagliatelle // YAML wire format uses snake_case.
	MaxModelCalls      int    `yaml:"max_model_calls,omitempty"`      //nolint:tagliatelle // YAML wire format uses snake_case.
}

// SecurityRules groups policy rules by decision priority.
type SecurityRules struct {
	Deny  []SecurityRule `yaml:"deny,omitempty"`
	Ask   []SecurityRule `yaml:"ask,omitempty"`
	Allow []SecurityRule `yaml:"allow,omitempty"`
}

// SecurityRule is one stable, user-authored policy rule.
type SecurityRule struct {
	ID    string            `yaml:"id"`
	Match SecurityRuleMatch `yaml:"match"`
}

// SecurityRuleMatch contains conjunctive selectors. Values within each list
// are alternatives.
type SecurityRuleMatch struct {
	Sources     []string `yaml:"sources,omitempty"`
	Deliveries  []string `yaml:"deliveries,omitempty"`
	Effects     []string `yaml:"effects,omitempty"`
	Paths       []string `yaml:"paths,omitempty"`
	Executables []string `yaml:"executables,omitempty"`
	ArgvPrefix  []string `yaml:"argv_prefix,omitempty"` //nolint:tagliatelle // YAML wire format uses snake_case.
	Certainty   []string `yaml:"certainty,omitempty"`
	Risk        []string `yaml:"risk,omitempty"`
}

// ProviderConfig stores one named LLM provider profile.
//
//nolint:tagliatelle // The YAML wire format uses snake_case.
type ProviderConfig struct {
	Type        string            `yaml:"type"`
	APIKey      string            `yaml:"api_key,omitempty"`
	Model       string            `yaml:"model"`
	BaseURL     string            `yaml:"base_url"`
	HTTPHeaders map[string]string `yaml:"http_headers,omitempty"`
}

// Config contains provider, HTTP, and command execution settings.
//
//nolint:tagliatelle // The YAML wire format uses snake_case.
type Config struct {
	original *projection

	Version        int                       `yaml:"version"`
	ActiveProvider string                    `yaml:"active_provider"`
	Providers      map[string]ProviderConfig `yaml:"providers"`
	Security       SecurityConfig            `yaml:"security"`
	UILanguage     string                    `yaml:"language"`
	HTTPHeaders    map[string]string         `yaml:"http_headers"`

	// The flattened provider fields remain an in-memory compatibility projection
	// for existing adapters. They are never written to YAML.
	LLMProvider string `yaml:"-"`

	OpenAIAPIKey  string `yaml:"-"`
	OpenAIModel   string `yaml:"-"`
	OpenAIBaseURL string `yaml:"-"`
	GeminiAPIKey  string `yaml:"-"`
	GeminiModel   string `yaml:"-"`
	GeminiBaseURL string `yaml:"-"`

	AnthropicAPIKey  string `yaml:"-"`
	AnthropicModel   string `yaml:"-"`
	AnthropicBaseURL string `yaml:"-"`

	OllamaBaseURL string `yaml:"-"`
	OllamaModel   string `yaml:"-"`

	CommandExecutionMode  string `yaml:"command_execution_mode"`
	CommandWrapperEnabled bool   `yaml:"command_wrapper_enabled"`
	ScriptEditor          string `yaml:"script_editor"`
	HistoryEnabled        bool   `yaml:"history_enabled"`
	ContextEnabled        bool   `yaml:"context_enabled"`
	ToolsEnabled          bool   `yaml:"tools_enabled"`
	ToolApprovalMode      string `yaml:"tool_approval_mode"`

	HistorySemanticEnabled     bool   `yaml:"history_semantic_enabled"`
	HistoryEmbeddingProvider   string `yaml:"history_embedding_provider,omitempty"`
	HistoryEmbeddingBaseURL    string `yaml:"history_embedding_base_url,omitempty"`
	HistoryEmbeddingModel      string `yaml:"history_embedding_model,omitempty"`
	HistoryEmbeddingDimensions int    `yaml:"history_embedding_dimensions,omitempty"`
	HistoryEmbeddingAPIKey     string `yaml:"history_embedding_api_key,omitempty"`
	HistoryEmbeddingMaxBytes   int    `yaml:"history_embedding_max_bytes"`
}

// DefaultConfig returns a new configuration with default provider and CLI settings.
func DefaultConfig() *Config {
	cfg := &Config{
		Version:        currentConfigVersion,
		ActiveProvider: kindOpenAI,
		Providers:      make(map[string]ProviderConfig),
		Security:       SecurityConfig{Version: 1, Mode: SecurityModeAuto, DefaultDecision: securityDecisionAsk, Probe: SecurityStageConfig{Mode: securityStageLocal, OnFailure: securityDecisionAsk}, Classifier: SecurityStageConfig{Mode: securityStageLocal, LocalActions: securityStageLocal, OnFailure: securityDecisionAsk}},
		UILanguage:     uiLanguageAuto,
		HTTPHeaders: map[string]string{
			"User-Agent": "zev/1.0",
		},
		OpenAIModel:              "gpt-5.6-luna",
		OpenAIBaseURL:            "https://api.openai.com/v1",
		GeminiModel:              "gemini-3.1-flash-lite",
		GeminiBaseURL:            "https://generativelanguage.googleapis.com",
		AnthropicModel:           "claude-haiku-5.5",
		AnthropicBaseURL:         "https://api.anthropic.com/v1",
		CommandExecutionMode:     "inject",
		ScriptEditor:             "nano",
		HistoryEnabled:           true,
		ContextEnabled:           true,
		ToolsEnabled:             true,
		ToolApprovalMode:         "manual",
		CommandWrapperEnabled:    true,
		HistoryEmbeddingMaxBytes: 32768,
	}
	cfg.LLMProvider = cfg.ActiveProvider
	cfg.Providers[cfg.ActiveProvider] = legacyProvider(cfg, cfg.ActiveProvider)
	cfg.captureProjection()
	return cfg
}

// ProviderNames returns the configured names in lexical order.
func (c *Config) ProviderNames() []string {
	if c == nil {
		return nil
	}
	return slices.Sorted(maps.Keys(c.Providers))
}

// ActiveProviderConfig returns the selected profile with a detached header map.
func (c *Config) ActiveProviderConfig() (ProviderConfig, error) {
	if c == nil {
		return ProviderConfig{}, errMissingActive
	}
	p, ok := c.Providers[c.ActiveProvider]
	if !ok || c.ActiveProvider == "" {
		return ProviderConfig{}, errMissingActive
	}
	if !supported(p.Type) {
		return ProviderConfig{}, errProviderType
	}
	p.HTTPHeaders = maps.Clone(p.HTTPHeaders)
	return p, nil
}

// EffectiveHTTPHeaders returns global request headers overridden by the active
// profile's headers. Neither configuration map is modified.
func (c *Config) EffectiveHTTPHeaders() map[string]string {
	headers := make(map[string]string)
	for _, m := range []map[string]string{c.HTTPHeaders, c.Providers[c.ActiveProvider].HTTPHeaders} {
		for _, k := range slices.Sorted(maps.Keys(m)) {
			headers[http.CanonicalHeaderKey(k)] = m[k]
		}
	}
	return headers
}

var (
	errMissingActive     = errors.New("active provider is not configured")
	errProviderType      = errors.New("unsupported provider type")
	errInvalidToolPolicy = errors.New("invalid tool policy")
	errInvalidUILanguage = errors.New("invalid UI language: must be auto, en, or ru")
)

func supported(kind string) bool {
	switch kind {
	case kindOpenAI, kindGemini, kindAnthropic, kindOllama, kindOpenCode:
		return true
	}
	return false
}

// Validate checks UI language, tool policy, and opt-in history embedding settings.
// Empty UI language means auto. Disabled semantic search deliberately ignores the
// other embedding fields. Invalid UI language errors never include its value.
func (c *Config) Validate() error {
	if c == nil {
		return nil
	}
	if _, err := c.ActiveProviderConfig(); err != nil {
		return err
	}
	for name, p := range c.Providers {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name || strings.ContainsFunc(name, unicode.IsControl) {
			return errors.New("invalid provider name")
		}
		if !supported(p.Type) {
			return errProviderType
		}
	}
	c.UILanguage = normalizeUILanguage(c.UILanguage)
	if err := validateUILanguage(c.UILanguage); err != nil {
		return err
	}
	if c.ToolApprovalMode != "manual" && c.ToolApprovalMode != "auto" && c.ToolApprovalMode != "bypass" {
		return fmt.Errorf("%w: approval mode must be manual, auto or bypass", errInvalidToolPolicy)
	}
	if err := c.Security.Validate(); err != nil {
		return fmt.Errorf("%w: %w", errInvalidToolPolicy, err)
	}
	if !c.HistorySemanticEnabled {
		return nil
	}
	if c.HistoryEmbeddingProvider != "openai-compatible" {
		return errors.New("unsupported history embedding provider")
	}
	if strings.TrimSpace(c.HistoryEmbeddingBaseURL) == "" {
		return errors.New("history embedding base URL is required")
	}
	if strings.TrimSpace(c.HistoryEmbeddingModel) == "" {
		return errors.New("history embedding model is required")
	}
	if c.HistoryEmbeddingDimensions <= 0 {
		return errors.New("history embedding dimensions must be positive")
	}
	if c.HistoryEmbeddingMaxBytes <= 0 {
		return errors.New("history embedding max bytes must be positive")
	}
	return nil
}

// Validate checks the version-one security policy schema before dispatch.
//
//nolint:gocognit // Validation keeps the fail-closed policy schema in one boundary.
func (c *SecurityConfig) Validate() error {
	if c == nil {
		return errors.New("security configuration is missing")
	}
	if c.Version == 0 {
		c.Version = 1
	}
	if c.Version != 1 {
		return errors.New("security version must be 1")
	}
	if c.Mode == "" {
		c.Mode = SecurityModeAuto
	}
	if c.Mode == "normal" {
		c.Mode = SecurityModeAuto
	}
	if c.Mode != SecurityModeAuto && c.Mode != SecurityModeBypass {
		return errors.New("security mode must be auto or bypass")
	}
	if c.DefaultDecision == "" {
		c.DefaultDecision = securityDecisionAsk
	}
	if c.DefaultDecision != securityDecisionAsk && c.DefaultDecision != "deny" {
		return errors.New("security default decision must be ask or deny")
	}
	if err := validateSecurityStage(&c.Probe, false); err != nil {
		return fmt.Errorf("probe: %w", err)
	}
	if err := validateSecurityStage(&c.Classifier, true); err != nil {
		return fmt.Errorf("classifier: %w", err)
	}

	for name, value := range map[string]string{"review_timeout": c.Limits.ReviewTimeout, "total_review_timeout": c.Limits.TotalReviewTimeout} {
		if value == "" {
			continue
		}
		if duration, err := time.ParseDuration(value); err != nil || duration <= 0 {
			return fmt.Errorf("security %s must be a positive duration", name)
		}
	}
	if c.Limits.MaxModelCalls < 0 {
		return errors.New("security max_model_calls cannot be negative")
	}
	for _, values := range [][]string{c.DangerousCommands.Add, c.DangerousCommands.Remove, c.DeniedCommands} {
		for _, command := range values {
			if command == "" || strings.TrimSpace(command) != command || len(command) > 4096 || strings.ContainsRune(command, 0) || strings.ContainsFunc(command, unicode.IsControl) {
				return errors.New("security command entries must be nonempty, trimmed, and contain no control characters")
			}
		}
	}
	return validateSecurityRules(c.Rules)
}

// CommandOverrides returns a detached runtime copy of command classification overrides.
func (c SecurityConfig) CommandOverrides() security.CommandOverrides {
	return security.CommandOverrides{
		Add: append([]string(nil), c.DangerousCommands.Add...), Remove: append([]string(nil), c.DangerousCommands.Remove...),
	}
}

func validateSecurityStage(stage *SecurityStageConfig, classifier bool) error {
	if stage.Mode == "" {
		stage.Mode = securityStageLocal
	}
	if stage.Mode != securityStageLocal {
		return errors.New("mode must be local in version 1")
	}
	if stage.OnFailure == "" {
		stage.OnFailure = securityDecisionAsk
	}
	if stage.OnFailure != securityDecisionAsk && stage.OnFailure != "deny" {
		return errors.New("on_failure must be ask or deny")
	}
	if classifier {
		if stage.LocalActions == "" {
			stage.LocalActions = securityStageLocal
		}
		if stage.LocalActions != securityStageLocal {
			return errors.New("local_actions must be local in version 1")
		}
	}
	return nil
}

func validateSecurityRules(rules SecurityRules) error {
	ids := make(map[string]bool)
	for _, group := range []struct {
		items []SecurityRule
	}{{rules.Deny}, {rules.Ask}, {rules.Allow}} {
		for _, rule := range group.items {
			if strings.TrimSpace(rule.ID) == "" || ids[rule.ID] {
				return errors.New("security rule IDs must be nonempty and unique")
			}
			ids[rule.ID] = true
			if emptySecurityMatch(rule.Match) {
				return errors.New("security rule match must not be empty")
			}
			if err := validateSecurityMatch(rule.Match); err != nil {
				return fmt.Errorf("security rule match: %w", err)
			}
		}
	}
	return nil
}

func emptySecurityMatch(match SecurityRuleMatch) bool {
	return len(match.Sources) == 0 && len(match.Deliveries) == 0 && len(match.Effects) == 0 && len(match.Paths) == 0 && len(match.Executables) == 0 && len(match.ArgvPrefix) == 0 && len(match.Certainty) == 0 && len(match.Risk) == 0
}

func validateSecurityMatch(match SecurityRuleMatch) error {
	if err := validateSecurityValues(match.Sources, map[string]bool{"generated": true, "context_tool": true, "query": true, "run": true, "alias": true, "script": true, "tool": true, "web_fetch": true}); err != nil {
		return errors.New("invalid source")
	}
	if err := validateSecurityValues(match.Deliveries, map[string]bool{"inject": true, "execute": true}); err != nil {
		return errors.New("invalid delivery")
	}
	if err := validateSecurityValues(match.Effects, map[string]bool{"read": true, "write": true, "delete": true, "process": true, "network": true, "terminal": true, "security_policy_change": true, "remote_write": true, "remote_history_rewrite": true}); err != nil {
		return errors.New("invalid effect")
	}
	if err := validateSecurityValues(match.Certainty, map[string]bool{"unknown": true, "partial": true, "complete": true}); err != nil {
		return errors.New("invalid certainty")
	}
	if err := validateSecurityValues(match.Risk, map[string]bool{"low": true, "medium": true, "high": true, "critical": true}); err != nil {
		return errors.New("invalid risk")
	}
	for _, value := range append(append([]string{}, match.Paths...), append(match.Executables, match.ArgvPrefix...)...) {
		if value == "" || strings.ContainsRune(value, 0) {
			return errors.New("empty or invalid selector")
		}
	}
	return nil
}

func validateSecurityValues(values []string, allowed map[string]bool) error {
	for _, value := range values {
		if value == "" || !allowed[value] {
			return errors.New("unsupported selector")
		}
	}
	return nil
}

// Policy converts the validated YAML policy into the runtime decision engine.
func (c SecurityConfig) Policy() security.Policy {
	policy := security.Policy{Mode: security.Mode(c.Mode), DefaultDecision: security.Decision(c.DefaultDecision), DeniedCommands: append([]string(nil), c.DeniedCommands...)}
	for decision, rules := range map[security.Decision][]SecurityRule{
		security.DecisionDeny:  c.Rules.Deny,
		security.DecisionAsk:   c.Rules.Ask,
		security.DecisionAllow: c.Rules.Allow,
	} {
		for _, rule := range rules {
			policy.Rules = append(policy.Rules, security.Rule{
				ID: rule.ID, Decision: decision,
				Sources: mapSources(rule.Match.Sources), Deliveries: mapDeliveries(rule.Match.Deliveries),
				Effects: mapEffects(rule.Match.Effects), Paths: append([]string(nil), rule.Match.Paths...),
				Executables: append([]string(nil), rule.Match.Executables...), ArgvPrefix: append([]string(nil), rule.Match.ArgvPrefix...),
				Certainty: mapCertainties(rule.Match.Certainty), Risks: mapRisks(rule.Match.Risk),
			})
		}
	}
	return policy
}

func mapSources(values []string) []security.Source {
	result := make([]security.Source, len(values))
	for i, value := range values {
		result[i] = security.Source(value)
	}
	return result
}

func mapDeliveries(values []string) []security.Delivery {
	result := make([]security.Delivery, len(values))
	for i, value := range values {
		result[i] = security.Delivery(value)
	}
	return result
}

func mapEffects(values []string) []security.Effect {
	result := make([]security.Effect, len(values))
	for i, value := range values {
		result[i] = security.Effect(value)
	}
	return result
}

func mapCertainties(values []string) []security.Certainty {
	result := make([]security.Certainty, len(values))
	for i, value := range values {
		result[i] = security.Certainty(value)
	}
	return result
}

func mapRisks(values []string) []security.Risk {
	result := make([]security.Risk, len(values))
	for i, value := range values {
		result[i] = security.Risk(value)
	}
	return result
}

const (
	kindOpenAI    = "openai"
	kindOpenCode  = "opencode"
	kindGemini    = "gemini"
	kindOllama    = "ollama"
	kindAnthropic = "anthropic"
)
