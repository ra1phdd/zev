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
