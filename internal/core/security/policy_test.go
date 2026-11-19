package security

import "testing"

func TestAssessInjectIsWarningOnly(t *testing.T) {
	t.Parallel()
	decision := Assess(ModeNormal, Action{Delivery: DeliveryInject, Dangerous: true})
	if decision != DecisionAllow {
		t.Fatalf("decision = %q", decision)
	}
}

func TestAssessExecuteAsksForDangerousAction(t *testing.T) {
	t.Parallel()
	decision := Assess(ModeNormal, Action{Delivery: DeliveryExecute, Dangerous: true})
	if decision != DecisionAsk {
		t.Fatalf("decision = %q", decision)
	}
}

func TestAssessNormalExecuteAsksUntilAnalysisIsComplete(t *testing.T) {
	t.Parallel()
	decision := Assess(ModeNormal, Action{Delivery: DeliveryExecute})
	if decision != DecisionAsk {
		t.Fatalf("decision = %q", decision)
	}
}

func TestAssessBypassAllowsDangerousAction(t *testing.T) {
	t.Parallel()
	decision := Assess(ModeBypass, Action{Delivery: DeliveryExecute, Dangerous: true})
	if decision != DecisionAllow {
		t.Fatalf("decision = %q", decision)
	}
}

func TestPolicyRejectsPartialAnalysisEvenWhenCallerMarksItKnown(t *testing.T) {
	t.Parallel()
	decision := (Policy{Mode: ModeNormal}).Evaluate(Action{
		Source:        SourceScript,
		Delivery:      DeliveryExecute,
		AnalysisKnown: true,
		Certainty:     CertaintyPartial,
	})
	if decision != DecisionAsk {
		t.Fatalf("decision = %q", decision)
	}
}

func TestPolicySupportsConservativeDefaultDeny(t *testing.T) {
	t.Parallel()
	decision := (Policy{Mode: ModeNormal, DefaultDecision: DecisionDeny}).Evaluate(Action{
		Source:        SourceTool,
		Delivery:      DeliveryExecute,
		AnalysisKnown: false,
		Certainty:     CertaintyUnknown,
	})
	if decision != DecisionDeny {
		t.Fatalf("decision = %q", decision)
	}
}

func TestPolicyRulePriorityIsIndependentOfDeclarationOrder(t *testing.T) {
	t.Parallel()
	policy := Policy{Mode: ModeNormal, Rules: []Rule{
		{ID: "allow", Decision: DecisionAllow, Sources: []Source{SourceTool}, Effects: []Effect{EffectRead}},
		{ID: "ask", Decision: DecisionAsk, Sources: []Source{SourceTool}, Effects: []Effect{EffectRead}},
		{ID: "deny", Decision: DecisionDeny, Sources: []Source{SourceTool}, Effects: []Effect{EffectRead}},
	}}
	decision := policy.Evaluate(Action{Source: SourceTool, Delivery: DeliveryExecute, Effects: []Effect{EffectRead}, AnalysisKnown: true, Certainty: CertaintyComplete})
	if decision != DecisionDeny {
		t.Fatalf("decision = %q", decision)
	}
}

func TestPolicyBypassCannotRemoveConfiguredDeny(t *testing.T) {
	t.Parallel()
	policy := Policy{Mode: ModeBypass, Rules: []Rule{{Decision: DecisionDeny}}}
	decision := policy.Evaluate(Action{Delivery: DeliveryExecute, AnalysisKnown: true, Certainty: CertaintyComplete})
	if decision != DecisionDeny {
		t.Fatalf("decision = %q", decision)
	}
}

func TestPolicyMatchesComponentAwareWorkspaceGlobs(t *testing.T) {
	t.Parallel()
	policy := Policy{Mode: ModeNormal, Rules: []Rule{{Decision: DecisionDeny, Paths: []string{"${workspace}/**"}}}}.WithContext("/workspace", "/home/user")
	for _, path := range []string{"/workspace/file.txt", "/workspace/nested/file.txt"} {
		if got := policy.Evaluate(Action{Delivery: DeliveryExecute, Paths: []string{path}, AnalysisKnown: true, Certainty: CertaintyComplete}); got != DecisionDeny {
			t.Fatalf("path %q decision = %q", path, got)
		}
	}
	if got := policy.Evaluate(Action{Delivery: DeliveryExecute, Paths: []string{"/other/file.txt"}, AnalysisKnown: true, Certainty: CertaintyComplete}); got != DecisionAllow {
		t.Fatalf("outside path decision = %q", got)
	}
}

func TestPolicyPathStarDoesNotCrossComponents(t *testing.T) {
	t.Parallel()
	policy := Policy{Mode: ModeNormal, Rules: []Rule{{Decision: DecisionDeny, Paths: []string{"/workspace/*"}}}}
	if got := policy.Evaluate(Action{Delivery: DeliveryExecute, Paths: []string{"/workspace/nested/file"}, AnalysisKnown: true, Certainty: CertaintyComplete}); got != DecisionAllow {
		t.Fatalf("nested path decision = %q", got)
	}
}

func TestPolicyAllowRuleMustCoverEveryEffect(t *testing.T) {
	t.Parallel()
	policy := Policy{Mode: ModeNormal, Rules: []Rule{{Decision: DecisionAllow, Effects: []Effect{EffectRead}}}}
	decision := policy.Evaluate(Action{Delivery: DeliveryExecute, Effects: []Effect{EffectRead, EffectNetwork}, AnalysisKnown: true, Certainty: CertaintyComplete})
	if decision != DecisionAsk {
		t.Fatalf("decision = %q", decision)
	}
}

func TestPolicyDeniedCommandCannotBeApprovedOrBypassed(t *testing.T) {
	t.Parallel()
	for _, mode := range []Mode{ModeAuto, ModeBypass} {
		policy := Policy{Mode: mode, DeniedCommands: []string{testForcePush}}
		got := policy.Evaluate(Action{Delivery: DeliveryExecute, Command: testForcePush})
		if got != DecisionDeny {
			t.Fatalf("mode %q allowed remembered denied command: %q", mode, got)
		}
	}
}
