package config

import "testing"

func TestSecurityPolicySchemaRoundTripsNestedRules(t *testing.T) {
	t.Parallel()
	cfg, err := Parse([]byte(`version: 1
active_provider: openai
providers:
  openai:
    type: openai
security:
  version: 1
  mode: auto
  default_decision: deny
  probe:
    mode: local
    on_failure: deny
  classifier:
    mode: local
    local_actions: local
    on_failure: ask
  limits:
    review_timeout: 15s
    total_review_timeout: 60s
    max_model_calls: 24
  rules:
    deny:
      - id: no-policy-self-edit
        match:
          sources: [generated]
          effects: [security_policy_change]
    ask:
      - id: remote-write
        match:
          effects: [remote_write]
          certainty: [unknown]
    allow:
      - id: project-read
        match:
          sources: [context_tool]
          effects: [read]
          paths: ["${workspace}/**"]
          certainty: [complete]
          risk: [low]
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Security.DefaultDecision != "deny" || len(cfg.Security.Rules.Allow) != 1 || cfg.Security.Limits.MaxModelCalls != 24 {
		t.Fatalf("security policy = %+v", cfg.Security)
	}
	policy := cfg.Security.Policy()
	if len(policy.Rules) != 3 || policy.DefaultDecision != "deny" {
		t.Fatalf("runtime policy = %+v", policy)
	}
}

func TestSecurityPolicySchemaRejectsDuplicateRuleIDsAndUnsupportedSandbox(t *testing.T) {
	t.Parallel()
	base := `version: 1
active_provider: openai
providers:
  openai:
    type: openai
security:
  version: 1
  rules:
    deny:
      - id: duplicate
        match: {effects: [read]}
    ask:
      - id: duplicate
        match: {effects: [write]}
`
	if _, err := Parse([]byte(base)); err == nil {
		t.Fatal("duplicate rule ID was accepted")
	}
	unsupported := `version: 1
active_provider: openai
providers:
  openai:
    type: openai
security:
  sandbox: {mode: required}
`
	if _, err := Parse([]byte(unsupported)); err == nil {
		t.Fatal("unsupported sandbox mode was accepted")
	}
}

func TestSecurityCommandOverridesRoundTrip(t *testing.T) {
	t.Parallel()
	cfg, err := Parse([]byte(`version: 1
active_provider: openai
providers:
  openai:
    type: openai
security:
  dangerous_commands:
    add: ["deploy --force"]
    remove: ["rm -rf ./tmp"]
  denied_commands: ["git push --force origin main"]
`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := encodeYAML(cfg)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(roundTrip.Security.DangerousCommands.Add) != 1 || len(roundTrip.Security.DangerousCommands.Remove) != 1 || len(roundTrip.Security.DeniedCommands) != 1 {
		t.Fatalf("security command overrides did not round-trip: %+v", roundTrip.Security)
	}
}

func TestSecurityCommandOverridesRejectEmptyEntries(t *testing.T) {
	t.Parallel()
	for _, yaml := range []string{
		`dangerous_commands: {add: [""]}`,
		`dangerous_commands: {remove: [""]}`,
		`denied_commands: [""]`,
	} {
		t.Run(yaml, func(t *testing.T) {
			input := "version: 1\nactive_provider: openai\nproviders:\n  openai:\n    type: openai\nsecurity:\n  " + yaml + "\n"
			if _, err := Parse([]byte(input)); err == nil {
				t.Fatal("empty command override was accepted")
			}
		})
	}
}
