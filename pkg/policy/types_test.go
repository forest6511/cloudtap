package policy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse_ValidPolicy(t *testing.T) {
	yaml := `
default: deny
rules:
  - name: allow-dev
    effect: allow
    match:
      target: "staging-*"
      user: "dev-*"
      source_ip: "10.0.0.0/8"
`
	policy, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if policy.Default != EffectDeny {
		t.Errorf("Default = %v, want %v", policy.Default, EffectDeny)
	}

	if len(policy.Rules) != 1 {
		t.Fatalf("len(Rules) = %d, want 1", len(policy.Rules))
	}

	rule := policy.Rules[0]
	if rule.Name != "allow-dev" {
		t.Errorf("Rule.Name = %v, want %v", rule.Name, "allow-dev")
	}
	if rule.Effect != EffectAllow {
		t.Errorf("Rule.Effect = %v, want %v", rule.Effect, EffectAllow)
	}
	if rule.Match.Target != "staging-*" {
		t.Errorf("Rule.Match.Target = %v, want %v", rule.Match.Target, "staging-*")
	}
	if rule.Match.User != "dev-*" {
		t.Errorf("Rule.Match.User = %v, want %v", rule.Match.User, "dev-*")
	}
	if rule.Match.SourceIP != "10.0.0.0/8" {
		t.Errorf("Rule.Match.SourceIP = %v, want %v", rule.Match.SourceIP, "10.0.0.0/8")
	}
}

func TestParse_DefaultAllow(t *testing.T) {
	yaml := `
default: allow
rules: []
`
	policy, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if policy.Default != EffectAllow {
		t.Errorf("Default = %v, want %v", policy.Default, EffectAllow)
	}
}

func TestParse_DefaultsToDefaultDeny(t *testing.T) {
	yaml := `
rules: []
`
	policy, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if policy.Default != EffectDeny {
		t.Errorf("Default = %v, want %v", policy.Default, EffectDeny)
	}
}

func TestParse_MultipleRules(t *testing.T) {
	yaml := `
default: deny
rules:
  - name: rule1
    effect: allow
    match:
      target: "staging-*"
  - name: rule2
    effect: deny
    match:
      user: "blocked-*"
  - name: rule3
    effect: allow
    match:
      source_ip: "192.168.0.0/16"
`
	policy, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(policy.Rules) != 3 {
		t.Fatalf("len(Rules) = %d, want 3", len(policy.Rules))
	}

	// Verify rule names
	expectedNames := []string{"rule1", "rule2", "rule3"}
	for i, name := range expectedNames {
		if policy.Rules[i].Name != name {
			t.Errorf("Rules[%d].Name = %v, want %v", i, policy.Rules[i].Name, name)
		}
	}
}

func TestParse_InvalidDefaultEffect(t *testing.T) {
	yaml := `
default: maybe
rules: []
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("Parse() expected error for invalid default effect")
	}
}

func TestParse_InvalidRuleEffect(t *testing.T) {
	yaml := `
default: deny
rules:
  - name: bad-rule
    effect: perhaps
    match:
      target: "*"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("Parse() expected error for invalid rule effect")
	}
}

func TestParse_MissingRuleName(t *testing.T) {
	yaml := `
default: deny
rules:
  - effect: allow
    match:
      target: "*"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("Parse() expected error for missing rule name")
	}
}

func TestParse_DuplicateRuleName(t *testing.T) {
	yaml := `
default: deny
rules:
  - name: duplicate
    effect: allow
    match:
      target: "a"
  - name: duplicate
    effect: deny
    match:
      target: "b"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("Parse() expected error for duplicate rule name")
	}
}

func TestParse_InvalidSourceIPCIDR(t *testing.T) {
	yaml := `
default: deny
rules:
  - name: bad-cidr
    effect: allow
    match:
      source_ip: "not-a-cidr"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("Parse() expected error for invalid CIDR")
	}
}

func TestParse_InvalidSourceIPCIDR_BadMask(t *testing.T) {
	yaml := `
default: deny
rules:
  - name: bad-mask
    effect: allow
    match:
      source_ip: "10.0.0.0/99"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("Parse() expected error for invalid CIDR mask")
	}
}

func TestParse_InvalidProtocol(t *testing.T) {
	yaml := `
default: deny
rules:
  - name: bad-proto
    effect: allow
    match:
      protocol: "ftp"
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("Parse() expected error for invalid protocol")
	}
}

func TestParse_ValidProtocols(t *testing.T) {
	protocols := []string{"tcp", "udp", "http", "https", "stcp", "sudp", "xtcp"}

	for _, proto := range protocols {
		yaml := `
default: deny
rules:
  - name: proto-rule
    effect: allow
    match:
      protocol: "` + proto + `"
`
		_, err := Parse([]byte(yaml))
		if err != nil {
			t.Errorf("Parse() error for valid protocol %q: %v", proto, err)
		}
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	yaml := `
this is not: valid: yaml: at: all
  - broken
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("Parse() expected error for invalid YAML")
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create temp file
	content := `
default: deny
rules:
  - name: test-rule
    effect: allow
    match:
      target: "test-*"
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "policy.yaml")

	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	policy, err := LoadFromFile(tmpFile)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	if len(policy.Rules) != 1 {
		t.Errorf("len(Rules) = %d, want 1", len(policy.Rules))
	}
	if policy.Rules[0].Name != "test-rule" {
		t.Errorf("Rule.Name = %v, want %v", policy.Rules[0].Name, "test-rule")
	}
}

func TestLoadFromFile_NotFound(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/path/policy.yaml")
	if err == nil {
		t.Fatal("LoadFromFile() expected error for nonexistent file")
	}
}

func TestDefaultPolicy(t *testing.T) {
	policy := DefaultPolicy()

	if policy.Default != EffectDeny {
		t.Errorf("Default = %v, want %v", policy.Default, EffectDeny)
	}
	if len(policy.Rules) != 0 {
		t.Errorf("len(Rules) = %d, want 0", len(policy.Rules))
	}
}

func TestMatch_IsEmpty(t *testing.T) {
	tests := []struct {
		name  string
		match Match
		want  bool
	}{
		{
			name:  "empty match",
			match: Match{},
			want:  true,
		},
		{
			name:  "with target",
			match: Match{Target: "test"},
			want:  false,
		},
		{
			name:  "with user",
			match: Match{User: "test"},
			want:  false,
		},
		{
			name:  "with source_ip",
			match: Match{SourceIP: "10.0.0.0/8"},
			want:  false,
		},
		{
			name:  "with protocol",
			match: Match{Protocol: "tcp"},
			want:  false,
		},
		{
			name:  "with all fields",
			match: Match{Target: "t", User: "u", SourceIP: "10.0.0.0/8", Protocol: "tcp"},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.match.IsEmpty(); got != tt.want {
				t.Errorf("Match.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPolicy_Validate(t *testing.T) {
	tests := []struct {
		name    string
		policy  Policy
		wantErr bool
	}{
		{
			name: "valid policy",
			policy: Policy{
				Default: EffectDeny,
				Rules: []Rule{
					{Name: "rule1", Effect: EffectAllow, Match: Match{Target: "*"}},
				},
			},
			wantErr: false,
		},
		{
			name: "empty rules is valid",
			policy: Policy{
				Default: EffectDeny,
				Rules:   []Rule{},
			},
			wantErr: false,
		},
		{
			name: "empty default uses deny",
			policy: Policy{
				Default: "",
				Rules:   []Rule{},
			},
			wantErr: false,
		},
		{
			name: "invalid default",
			policy: Policy{
				Default: "invalid",
				Rules:   []Rule{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Policy.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRule_Validate(t *testing.T) {
	tests := []struct {
		name    string
		rule    Rule
		wantErr bool
	}{
		{
			name:    "valid rule",
			rule:    Rule{Name: "test", Effect: EffectAllow, Match: Match{Target: "*"}},
			wantErr: false,
		},
		{
			name:    "missing name",
			rule:    Rule{Name: "", Effect: EffectAllow, Match: Match{Target: "*"}},
			wantErr: true,
		},
		{
			name:    "invalid effect",
			rule:    Rule{Name: "test", Effect: "invalid", Match: Match{Target: "*"}},
			wantErr: true,
		},
		{
			name:    "invalid match",
			rule:    Rule{Name: "test", Effect: EffectAllow, Match: Match{SourceIP: "invalid"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Rule.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadExamplePolicy(t *testing.T) {
	// Test that the example policy file is valid
	policy, err := LoadFromFile("../../examples/policy.yaml")
	if err != nil {
		t.Fatalf("LoadFromFile(examples/policy.yaml) error = %v", err)
	}

	if policy.Default != EffectDeny {
		t.Errorf("Default = %v, want %v", policy.Default, EffectDeny)
	}

	if len(policy.Rules) == 0 {
		t.Error("expected example policy to have rules")
	}

	// Verify some expected rules exist
	ruleNames := make(map[string]bool)
	for _, rule := range policy.Rules {
		ruleNames[rule.Name] = true
	}

	expectedRules := []string{"allow-dev-staging", "allow-admin-all", "allow-monitoring"}
	for _, name := range expectedRules {
		if !ruleNames[name] {
			t.Errorf("expected rule %q not found in example policy", name)
		}
	}
}
