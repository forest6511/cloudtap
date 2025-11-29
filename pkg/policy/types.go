// Package policy provides policy evaluation for cloudtap.
//
// cloudtap uses a default-deny policy model. Policies are defined in YAML
// configuration files and loaded at startup. Each policy contains rules
// that specify match conditions and effects (allow/deny).
//
// # Policy Structure
//
// A policy file has the following structure:
//
//	default: deny
//	rules:
//	  - name: allow-dev-staging
//	    effect: allow
//	    match:
//	      target: "staging-*"
//	      user: "dev-*"
//	      source_ip: "10.0.0.0/8"
//
// # Matching
//
// Rules are evaluated in order. The first matching rule determines the outcome.
// If no rule matches, the default effect is applied.
//
// Match conditions support:
//   - target: glob pattern matching against the target service name
//   - user: glob pattern matching against the authenticated user
//   - source_ip: CIDR notation for IP address matching
package policy

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Effect represents the outcome of a policy rule.
type Effect string

const (
	// EffectAllow permits the connection.
	EffectAllow Effect = "allow"
	// EffectDeny rejects the connection.
	EffectDeny Effect = "deny"
)

// Policy represents a complete policy configuration.
type Policy struct {
	// Default is the default effect when no rules match.
	// Must be "allow" or "deny". Defaults to "deny" if not specified.
	Default Effect `yaml:"default" json:"default"`

	// Rules is the ordered list of policy rules.
	// Rules are evaluated in order; first match wins.
	Rules []Rule `yaml:"rules" json:"rules"`
}

// Rule represents a single policy rule.
type Rule struct {
	// Name is a human-readable identifier for the rule.
	// Required and must be unique within the policy.
	Name string `yaml:"name" json:"name"`

	// Effect is the outcome when this rule matches.
	// Must be "allow" or "deny".
	Effect Effect `yaml:"effect" json:"effect"`

	// Match contains the conditions that must be satisfied.
	// All specified conditions must match (AND logic).
	Match Match `yaml:"match" json:"match"`
}

// Match contains the conditions for a rule to match.
// All specified (non-empty) fields must match for the rule to apply.
type Match struct {
	// Target is a glob pattern for matching the target service name.
	// Examples: "staging-*", "prod-web-*", "*-internal"
	Target string `yaml:"target,omitempty" json:"target,omitempty"`

	// User is a glob pattern for matching the authenticated user.
	// Examples: "admin", "dev-*", "*@example.com"
	User string `yaml:"user,omitempty" json:"user,omitempty"`

	// SourceIP is a CIDR notation for matching the source IP address.
	// Examples: "10.0.0.0/8", "192.168.1.0/24", "0.0.0.0/0"
	SourceIP string `yaml:"source_ip,omitempty" json:"source_ip,omitempty"`

	// Protocol matches the connection protocol.
	// Examples: "tcp", "http", "https"
	Protocol string `yaml:"protocol,omitempty" json:"protocol,omitempty"`
}

// Validate checks if the policy configuration is valid.
func (p *Policy) Validate() error {
	// Validate default effect
	if p.Default != "" && p.Default != EffectAllow && p.Default != EffectDeny {
		return fmt.Errorf("policy: invalid default effect %q, must be %q or %q",
			p.Default, EffectAllow, EffectDeny)
	}

	// Track rule names for uniqueness check
	names := make(map[string]bool)

	for i, rule := range p.Rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("policy: rule[%d]: %w", i, err)
		}

		if names[rule.Name] {
			return fmt.Errorf("policy: duplicate rule name %q", rule.Name)
		}
		names[rule.Name] = true
	}

	return nil
}

// Validate checks if the rule is valid.
func (r *Rule) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("name is required")
	}

	if r.Effect != EffectAllow && r.Effect != EffectDeny {
		return fmt.Errorf("invalid effect %q, must be %q or %q",
			r.Effect, EffectAllow, EffectDeny)
	}

	if err := r.Match.Validate(); err != nil {
		return fmt.Errorf("match: %w", err)
	}

	return nil
}

// Validate checks if the match conditions are valid.
func (m *Match) Validate() error {
	// Validate CIDR notation if specified
	if m.SourceIP != "" {
		_, _, err := net.ParseCIDR(m.SourceIP)
		if err != nil {
			return fmt.Errorf("invalid source_ip CIDR %q: %w", m.SourceIP, err)
		}
	}

	// Validate protocol if specified
	if m.Protocol != "" {
		valid := []string{"tcp", "udp", "http", "https", "stcp", "sudp", "xtcp"}
		found := false
		for _, v := range valid {
			if strings.EqualFold(m.Protocol, v) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("invalid protocol %q", m.Protocol)
		}
	}

	return nil
}

// IsEmpty returns true if no match conditions are specified.
func (m *Match) IsEmpty() bool {
	return m.Target == "" && m.User == "" && m.SourceIP == "" && m.Protocol == ""
}

// LoadFromFile loads a policy from a YAML file.
func LoadFromFile(path string) (*Policy, error) {
	// Clean and validate path
	path = filepath.Clean(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("policy: failed to read file %s: %w", path, err)
	}

	return Parse(data)
}

// Parse parses a policy from YAML bytes.
func Parse(data []byte) (*Policy, error) {
	var policy Policy

	if err := yaml.Unmarshal(data, &policy); err != nil {
		return nil, fmt.Errorf("policy: failed to parse YAML: %w", err)
	}

	// Apply defaults
	if policy.Default == "" {
		policy.Default = EffectDeny
	}

	// Validate
	if err := policy.Validate(); err != nil {
		return nil, err
	}

	return &policy, nil
}

// DefaultPolicy returns a default-deny policy with no rules.
func DefaultPolicy() *Policy {
	return &Policy{
		Default: EffectDeny,
		Rules:   []Rule{},
	}
}
