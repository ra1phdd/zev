// Package security applies local permission policy before command delivery.
package security

import (
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Mode selects the permission behavior for command delivery.
type Mode string

const (
	// ModeAuto applies deterministic permission decisions.
	ModeAuto Mode = "auto"
	// ModeNormal is a source-compatibility alias for ModeAuto.
	ModeNormal Mode = ModeAuto
	// ModeBypass allows every otherwise valid command-delivery action in this scope.
	ModeBypass Mode = "bypass"
)

// Delivery identifies how a command leaves Zev.
type Delivery string

// Supported command delivery modes.
const (
	DeliveryInject  Delivery = "inject"
	DeliveryExecute Delivery = "execute"
)

// Source identifies the subsystem requesting a security-sensitive action.
// Keeping the source in the action envelope prevents callers from silently
// applying a query-only policy to scripts, aliases, or tools.
type Source string

// Sources identify the subsystem requesting an action.
const (
	SourceQuery    Source = "query"
	SourceRun      Source = "run"
	SourceAlias    Source = "alias"
	SourceScript   Source = "script"
	SourceTool     Source = "tool"
	SourceWebFetch Source = "web_fetch"
)

// Certainty describes how complete the local analysis is.
type Certainty string

// Certainty values describe the completeness of local analysis.
const (
	CertaintyUnknown  Certainty = "unknown"
	CertaintyPartial  Certainty = "partial"
	CertaintyComplete Certainty = "complete"
)

// Risk is the policy-visible impact classification of an action.
type Risk string

// Risks classify the impact of an action.
const (
	RiskLow      Risk = "low"
	RiskMedium   Risk = "medium"
	RiskHigh     Risk = "high"
	RiskCritical Risk = "critical"
)

// Provenance identifies where an action came from and whether its bytes were
// verified before authorization. It is part of the action envelope so a grant
// cannot silently transfer from model output to a saved artifact.
type Provenance struct {
	Kind       string
	Identifier string
	Digest     string
	Trusted    bool
}

// Effect identifies a security-relevant side effect requested by an action.
type Effect string

// Effects identify security-relevant side effects.
const (
	EffectProcess              Effect = "process"
	EffectFilesystem           Effect = "filesystem"
	EffectNetwork              Effect = "network"
	EffectTerminal             Effect = "terminal"
	EffectRead                 Effect = "read"
	EffectWrite                Effect = "write"
	EffectDelete               Effect = "delete"
	EffectPolicyChange         Effect = "security_policy_change"
	EffectRemoteWrite          Effect = "remote_write"
	EffectRemoteHistoryRewrite Effect = "remote_history_rewrite"
)

// Decision is the result of the deterministic permission gate.
type Decision string

// Permission outcomes.
const (
	DecisionAllow Decision = "allow"
	DecisionAsk   Decision = "ask"
	DecisionDeny  Decision = "deny"
)

// Action is the minimum command information needed by the first local gate.
type Action struct {
	Provenance          Provenance
	Source              Source
	Delivery            Delivery
	Command             string
	Dangerous           bool
	AnalysisKnown       bool
	Certainty           Certainty
	Effects             []Effect
	Interpreter         string
	WorkingDir          string
	Executable          string
	Argv                []string
	Paths               []string
	NetworkDestinations []string
	Risk                Risk
}

// Rule is a conjunctive policy selector. Values within one selector are
// alternatives; matching rules are resolved by deny > ask > allow priority.
type Rule struct {
	ID          string
	Decision    Decision
	Sources     []Source
	Deliveries  []Delivery
	Effects     []Effect
	Paths       []string
	Executables []string
	ArgvPrefix  []string
	Certainty   []Certainty
	Risks       []Risk
}

// Policy is the single deterministic decision engine shared by command,
// script, alias, tool, and web-fetch dispatchers. Callers may add rules later;
// the default remains conservative and preserves the existing normal-mode
// inject warning semantics.
type Policy struct {
	Mode            Mode
	DefaultDecision Decision
	Rules           []Rule
	DeniedCommands  []string
	Workspace       string
	Home            string
}

// WithContext binds trusted invocation roots used to expand policy path selectors.
func (p Policy) WithContext(workspace, home string) Policy {
	p.Workspace = workspace
	p.Home = home
	return p
}

// Evaluate returns the decision for one complete action envelope. An execute
// action is never automatically allowed when its analysis is incomplete.
func (p Policy) Evaluate(action Action) Decision {
	if p.Mode != "" && p.Mode != ModeAuto && p.Mode != ModeBypass {
		return DecisionDeny
	}
	if action.Delivery != DeliveryInject && action.Delivery != DeliveryExecute {
		return DecisionDeny
	}
	if action.Delivery == DeliveryExecute && slices.Contains(p.DeniedCommands, action.Command) {
		return DecisionDeny
	}
	if action.Delivery == DeliveryInject {
		return DecisionAllow
	}
	matched := p.matchingDecisions(action)
	if matched[DecisionDeny] {
		return DecisionDeny
	}
	if p.Mode == ModeBypass {
		return DecisionAllow
	}
	if matched[DecisionAsk] {
		return DecisionAsk
	}
	if action.AnalysisKnown && !action.Dangerous && (action.Certainty == "" || action.Certainty == CertaintyComplete) {
		if p.DefaultDecision == DecisionDeny && !matched[DecisionAllow] {
			return DecisionDeny
		}
		return DecisionAllow
	}
	if p.DefaultDecision == DecisionDeny {
		return DecisionDeny
	}
	return DecisionAsk
}

func (p Policy) matchingDecisions(action Action) map[Decision]bool {
	matched := make(map[Decision]bool)
	for _, rule := range p.Rules {
		if rule.Decision != DecisionAllow && rule.Decision != DecisionAsk && rule.Decision != DecisionDeny {
			continue
		}
		if p.ruleMatches(rule, action) {
			matched[rule.Decision] = true
			continue
		}
		if rule.Decision == DecisionAllow && p.ruleContextMatches(rule, action) {
			// An allow rule that does not cover every requested effect creates
			// an uncertainty floor instead of becoming a partial allow.
			matched[DecisionAsk] = true
		}
	}
	return matched
}

func (p Policy) ruleMatches(rule Rule, action Action) bool {
	return matchesOne(rule.Sources, action.Source) && matchesOne(rule.Deliveries, action.Delivery) && matchesEffects(rule.Effects, action.Effects) && p.matchesPaths(rule.Paths, action.Paths) && matchesString(rule.Executables, action.Executable) && matchesPrefix(rule.ArgvPrefix, action.Argv) && matchesOne(rule.Certainty, action.Certainty) && matchesOne(rule.Risks, action.Risk)
}

func (p Policy) ruleContextMatches(rule Rule, action Action) bool {
	return matchesOne(rule.Sources, action.Source) && matchesOne(rule.Deliveries, action.Delivery) && p.matchesPaths(rule.Paths, action.Paths) && matchesString(rule.Executables, action.Executable) && matchesPrefix(rule.ArgvPrefix, action.Argv) && matchesOne(rule.Certainty, action.Certainty) && matchesOne(rule.Risks, action.Risk)
}

func matchesOne[T comparable](selectors []T, value T) bool {
	if len(selectors) == 0 {
		return true
	}
	return slices.Contains(selectors, value)
}

func matchesEffects(selectors, effects []Effect) bool {
	if len(selectors) == 0 {
		return true
	}
	if len(effects) == 0 {
		return false
	}
	for _, effect := range effects {
		if !matchesOne(selectors, effect) {
			return false
		}
	}
	return true
}

func (p Policy) matchesPaths(selectors, values []string) bool {
	if len(selectors) == 0 {
		return true
	}
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		matched := false
		for _, selector := range selectors {
			if matchPolicyPath(expandPolicyPath(selector, p), value) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func expandPolicyPath(selector string, policy Policy) string {
	selector = strings.ReplaceAll(selector, "${workspace}", filepath.ToSlash(policy.Workspace))
	return strings.ReplaceAll(selector, "${home}", filepath.ToSlash(policy.Home))
}

func matchPolicyPath(pattern, value string) bool {
	pattern = strings.TrimSuffix(filepath.ToSlash(pattern), "/")
	value = strings.TrimSuffix(filepath.ToSlash(value), "/")
	if pattern == "" || value == "" {
		return pattern == value
	}
	patterns, values := strings.Split(pattern, "/"), strings.Split(value, "/")
	cache := make(map[[2]int]bool)
	seen := make(map[[2]int]bool)
	var match func(int, int) bool
	match = func(pi, vi int) bool {
		key := [2]int{pi, vi}
		if seen[key] {
			return cache[key]
		}
		seen[key] = true
		result := false
		switch {
		case pi == len(patterns):
			result = vi == len(values)
		case patterns[pi] == "**":
			result = match(pi+1, vi) || vi < len(values) && match(pi, vi+1)
		case vi < len(values):
			segment, err := path.Match(patterns[pi], values[vi])
			result = err == nil && segment && match(pi+1, vi+1)
		}
		cache[key] = result
		return result
	}
	return match(0, 0)
}

func matchesString(selectors []string, value string) bool {
	if len(selectors) == 0 {
		return true
	}
	return slices.Contains(selectors, value)
}

func matchesPrefix(prefix, argv []string) bool {
	if len(prefix) == 0 {
		return true
	}
	if len(argv) < len(prefix) {
		return false
	}
	for i := range prefix {
		if argv[i] != prefix[i] {
			return false
		}
	}
	return true
}

// Assess returns the command-delivery decision. Normal execute mode is
// confirmation-gated until complete effect analysis is available; inject remains
// warning-only because execution is handed to the user's shell.
func Assess(mode Mode, action Action) Decision {
	return Policy{Mode: mode}.Evaluate(action)
}
