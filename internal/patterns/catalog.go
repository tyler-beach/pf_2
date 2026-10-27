// Package patterns provides the reusable verification pattern registry that
// obligation reviews bind to. adr/requirements.json remains the compatibility
// catalog that materializes accepted requirement/verifier definitions.
package patterns

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

const SchemaVersion = "1"

// Pattern is a reusable verification capability, not an ADR-specific contract.
type Pattern struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	VerifierTypes []string `json:"verifier_types"`
	Inputs        []string `json:"inputs,omitempty"`
	Applicability string   `json:"applicability,omitempty"`
	BlindSpots    []string `json:"blind_spots,omitempty"`
	FixtureRefs   []string `json:"fixture_refs,omitempty"`
}

// Catalog is the versioned pattern registry.
type Catalog struct {
	SchemaVersion string    `json:"schema_version"`
	Patterns      []Pattern `json:"patterns"`
	Digest        string    `json:"digest,omitempty"`
}

// Load reads and validates a pattern catalog, sealing a content digest.
func Load(path string) (Catalog, error) {
	var c Catalog
	data, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("read pattern catalog: %w", err)
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("decode pattern catalog: %w", err)
	}
	if err := Validate(c); err != nil {
		return c, err
	}
	c.Digest = ContentDigest(c)
	return c, nil
}

// Validate checks schema and uniqueness.
func Validate(c Catalog) error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported pattern catalog schema_version %q", c.SchemaVersion)
	}
	if len(c.Patterns) == 0 {
		return fmt.Errorf("pattern catalog must declare at least one pattern")
	}
	seen := make(map[string]bool, len(c.Patterns))
	for _, p := range c.Patterns {
		id := strings.TrimSpace(p.ID)
		if id == "" || strings.TrimSpace(p.Title) == "" {
			return fmt.Errorf("each pattern requires id and title")
		}
		if seen[id] {
			return fmt.Errorf("duplicate pattern id %q", id)
		}
		seen[id] = true
		if len(p.VerifierTypes) == 0 {
			return fmt.Errorf("pattern %q requires verifier_types", id)
		}
	}
	return nil
}

// ContentDigest hashes canonical pattern definitions (excluding Digest field).
func ContentDigest(c Catalog) string {
	payload := struct {
		SchemaVersion string    `json:"schema_version"`
		Patterns      []Pattern `json:"patterns"`
	}{SchemaVersion: c.SchemaVersion, Patterns: append([]Pattern(nil), c.Patterns...)}
	sort.Slice(payload.Patterns, func(i, j int) bool { return payload.Patterns[i].ID < payload.Patterns[j].ID })
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Lookup returns a pattern by ID.
func (c Catalog) Lookup(id string) (Pattern, bool) {
	id = strings.TrimSpace(id)
	for _, p := range c.Patterns {
		if p.ID == id {
			return p, true
		}
	}
	return Pattern{}, false
}

// CompatibleVerifier reports whether a verifier type is allowed for the pattern.
func (c Catalog) CompatibleVerifier(patternID, verifierType string) bool {
	p, ok := c.Lookup(patternID)
	if !ok {
		return false
	}
	for _, allowed := range p.VerifierTypes {
		if allowed == verifierType {
			return true
		}
	}
	return false
}

// Builtin returns the default in-repo pattern IDs used before a file is loaded.
func BuiltinIDs() []string {
	return []string{
		"configuration.bounds_or_required_values",
		"security.source_pattern",
		"architecture.dependency_or_boundary",
		"manual_review",
		"configuration.json_fact",
	}
}
