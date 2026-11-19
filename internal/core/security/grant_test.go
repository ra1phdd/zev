package security

import "testing"

func TestGrantBindsInvocationAndIsSingleUse(t *testing.T) {
	t.Parallel()
	const (
		path   = "PATH=/usr/bin"
		printf = "printf"
	)
	action := Action{
		Source:        SourceRun,
		Delivery:      DeliveryExecute,
		AnalysisKnown: true,
		Certainty:     CertaintyComplete,
		WorkingDir:    t.TempDir(),
		Executable:    printf,
		Argv:          []string{printf, "ok"},
	}
	policy := Policy{Mode: ModeNormal, DefaultDecision: DecisionAsk}
	environment := []string{path}
	grant, err := NewGrant(action, policy, environment, []byte("reviewed input"))
	if err != nil {
		t.Fatalf("NewGrant() error = %v", err)
	}
	if err := grant.Consume(action, policy, environment, []byte("reviewed input")); err != nil {
		t.Fatalf("first Consume() error = %v", err)
	}
	if err := grant.Consume(action, policy, environment, []byte("reviewed input")); err == nil {
		t.Fatal("second Consume() unexpectedly succeeded")
	}
}

func TestGrantRejectsChangedSnapshotDimensions(t *testing.T) {
	t.Parallel()
	const (
		pathUserBin = "PATH=/usr/bin"
		pathBin     = "PATH=/bin"
	)
	base := Action{
		Source:      SourceScript,
		Delivery:    DeliveryExecute,
		WorkingDir:  t.TempDir(),
		Interpreter: "bash",
		Executable:  "script",
		Argv:        []string{"script"},
	}
	policy := Policy{Mode: ModeNormal, DefaultDecision: DecisionAsk}
	grant, err := NewGrant(base, policy, []string{pathUserBin}, []byte("body-v1"))
	if err != nil {
		t.Fatalf("NewGrant() error = %v", err)
	}
	tests := []struct {
		name   string
		change func(Action, Policy, []string, []byte) (Action, Policy, []string, []byte)
	}{
		{"interpreter", func(a Action, p Policy, e []string, i []byte) (Action, Policy, []string, []byte) {
			a.Interpreter = "sh"
			return a, p, e, i
		}},
		{"policy", func(a Action, p Policy, e []string, i []byte) (Action, Policy, []string, []byte) {
			p.Mode = ModeBypass
			return a, p, e, i
		}},
		{"environment", func(a Action, p Policy, _ []string, i []byte) (Action, Policy, []string, []byte) {
			return a, p, []string{pathBin}, i
		}},
		{"script input", func(a Action, p Policy, e []string, _ []byte) (Action, Policy, []string, []byte) {
			return a, p, e, []byte("body-v2")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			action, changedPolicy, environment, input := test.change(base, policy, []string{pathUserBin}, []byte("body-v1"))
			if err := grant.Consume(action, changedPolicy, environment, input); err == nil {
				t.Fatal("Consume() unexpectedly accepted changed snapshot")
			}
		})
	}
}
