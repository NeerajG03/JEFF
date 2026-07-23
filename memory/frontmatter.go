// frontmatter.go — YAML frontmatter parser/writer for memory entries.
// Worker-facing schema (Frontmatter) is 3 fields: name, description, type.
// Canonical schema (CanonicalFrontmatter) is the marlowe-enriched superset.
package memory

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Frontmatter is the worker-facing schema. Mirrors Claude Code's three-field
// shape so workers don't have to learn JEFF-specific fields.
type Frontmatter struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Type        MemoryType `yaml:"type"`
}

// CanonicalFrontmatter is what marlowe writes into JEFF_HOME/memory/**.
// Only the curator (Worker C) writes these; FND defines the type so D can read.
type CanonicalFrontmatter struct {
	Frontmatter  `yaml:",inline"`
	Status       string     `yaml:"status"` // accepted | superseded
	Scope        string     `yaml:"scope"`  // persona:<x> | repo:<y> | project:<z>
	GoalServed   string     `yaml:"goal_served,omitempty"`
	Importance   int        `yaml:"importance,omitempty"`
	ValidFrom    time.Time  `yaml:"valid_from"`
	ValidTo      *time.Time `yaml:"valid_to,omitempty"`
	Supersedes   []string   `yaml:"supersedes,omitempty"`
	SupersededBy string     `yaml:"superseded_by,omitempty"`
	Verifier     *Verifier  `yaml:"verifier,omitempty"`
	Provenance   string     `yaml:"provenance,omitempty"` // trusted | review-required
	Source       Source     `yaml:"source"`
}

// Verifier records the result of a procedural-memory verifier gate.
type Verifier struct {
	Type   string    `yaml:"type"`   // script | llm-judge | none
	Result string    `yaml:"result"` // pass | fail | n/a
	RanAt  time.Time `yaml:"ran_at"`
}

// Source records where a canonical entry came from.
type Source struct {
	Persona string `yaml:"persona"`
	Task    string `yaml:"task"`
	Session string `yaml:"session,omitempty"`
	Trigger string `yaml:"trigger"` // user-correction | self-noted | sessionend
}

// Parse reads `---\n<yaml>\n---\n<body>` from r and returns the parsed
// frontmatter, the body string, and any error. The body preserves the
// original trailing newline (or lack thereof) after the closing fence.
func Parse(r io.Reader) (Frontmatter, string, error) {
	var fm Frontmatter
	yamlBlock, body, err := splitFrontmatter(r)
	if err != nil {
		return fm, "", err
	}
	if err := yaml.Unmarshal(yamlBlock, &fm); err != nil {
		return fm, "", fmt.Errorf("frontmatter: yaml decode: %w", err)
	}
	return fm, body, nil
}

// Write writes a memory file with frontmatter and body to w.
// Always emits a trailing newline after the body for POSIX cleanliness.
func Write(w io.Writer, fm Frontmatter, body string) error {
	if fm.Name == "" {
		return fmt.Errorf("frontmatter: name is required")
	}
	if !fm.Type.Valid() {
		return fmt.Errorf("frontmatter: invalid type %q", fm.Type)
	}
	return writeFrontmatter(w, fm, body)
}

// ParseCanonical reads the marlowe-enriched canonical schema from r.
func ParseCanonical(r io.Reader) (CanonicalFrontmatter, string, error) {
	var fm CanonicalFrontmatter
	yamlBlock, body, err := splitFrontmatter(r)
	if err != nil {
		return fm, "", err
	}
	if err := yaml.Unmarshal(yamlBlock, &fm); err != nil {
		return fm, "", fmt.Errorf("canonical frontmatter: yaml decode: %w", err)
	}
	return fm, body, nil
}

// writeCanonical serializes a canonical memory entry to w.
// Exported as WriteCanonical(home, scope, bucket, fm, body) in store.go for callers
// that want filesystem-backed writes; this is the low-level serializer.
func writeCanonicalFile(w io.Writer, fm CanonicalFrontmatter, body string) error {
	if fm.Name == "" {
		return fmt.Errorf("canonical frontmatter: name is required")
	}
	if !fm.Type.Valid() {
		return fmt.Errorf("canonical frontmatter: invalid type %q", fm.Type)
	}
	return writeFrontmatter(w, fm, body)
}

// ---- internal ----

// splitFrontmatter reads `---\n…\n---\n<body>` from r and returns the YAML
// bytes (without the fences) and the body.
func splitFrontmatter(r io.Reader) ([]byte, string, error) {
	br := bufio.NewReader(r)

	first, err := br.ReadString('\n')
	if err != nil {
		return nil, "", fmt.Errorf("frontmatter: read opening fence: %w", err)
	}
	if strings.TrimRight(first, "\r\n") != "---" {
		return nil, "", fmt.Errorf("frontmatter: missing opening --- fence")
	}

	var yamlBuf bytes.Buffer
	for {
		line, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, "", fmt.Errorf("frontmatter: read yaml: %w", err)
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "---" {
			break
		}
		yamlBuf.WriteString(line)
		if err == io.EOF {
			return nil, "", fmt.Errorf("frontmatter: missing closing --- fence")
		}
	}

	rest, err := io.ReadAll(br)
	if err != nil {
		return nil, "", fmt.Errorf("frontmatter: read body: %w", err)
	}
	body := string(rest)
	body = strings.TrimPrefix(body, "\n")

	return yamlBuf.Bytes(), body, nil
}

func writeFrontmatter(w io.Writer, v any, body string) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("frontmatter: yaml encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("frontmatter: yaml close: %w", err)
	}

	if _, err := io.WriteString(w, "---\n"); err != nil {
		return err
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "---\n"); err != nil {
		return err
	}
	if body == "" {
		return nil
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}
	if _, err := io.WriteString(w, body); err != nil {
		return err
	}
	if !strings.HasSuffix(body, "\n") {
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}
	return nil
}
