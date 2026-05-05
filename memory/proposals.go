// proposals.go — read/write helpers for proposals/<persona>/<task>/*.md.
// Workers (B) write here via `jeff memory propose`; marlowe (C) reads them
// during curation.
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Proposal is one worker-authored memory proposal on disk.
type Proposal struct {
	Persona string
	Task    string
	Slug    string // file basename without .md
	FM      Frontmatter
	Body    string
	Path    string
	Created time.Time
}

// WriteProposal writes a proposal file at proposals/<persona>/<task>/<slug>.md
// where <slug> = fm.Name. Returns the resulting Proposal struct.
func WriteProposal(jeffHome, persona, task string, fm Frontmatter, body string) (Proposal, error) {
	if persona == "" {
		return Proposal{}, fmt.Errorf("WriteProposal: persona is required")
	}
	if task == "" {
		return Proposal{}, fmt.Errorf("WriteProposal: task is required")
	}
	if fm.Name == "" {
		return Proposal{}, fmt.Errorf("WriteProposal: frontmatter.name is required")
	}
	if !fm.Type.Valid() {
		return Proposal{}, fmt.Errorf("WriteProposal: invalid type %q", fm.Type)
	}

	dir := ProposalsTaskPath(jeffHome, persona, task)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Proposal{}, fmt.Errorf("WriteProposal: mkdir: %w", err)
	}

	slug := fm.Name
	path := filepath.Join(dir, slug+".md")

	f, err := os.Create(path)
	if err != nil {
		return Proposal{}, fmt.Errorf("WriteProposal: create: %w", err)
	}
	defer f.Close()
	if err := Write(f, fm, body); err != nil {
		return Proposal{}, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return Proposal{}, fmt.Errorf("WriteProposal: stat: %w", err)
	}

	return Proposal{
		Persona: persona,
		Task:    task,
		Slug:    slug,
		FM:      fm,
		Body:    body,
		Path:    path,
		Created: info.ModTime(),
	}, nil
}

// ListProposals returns all proposals matching the (persona, task) filter.
// An empty persona means "any persona"; an empty task means "any task under
// the matching personas".
func ListProposals(jeffHome, persona, task string) ([]Proposal, error) {
	root := ProposalsRoot(jeffHome)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}

	var out []Proposal
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) < 3 {
			return nil
		}
		gotPersona, gotTask := parts[0], parts[1]
		if persona != "" && gotPersona != persona {
			return nil
		}
		if task != "" && gotTask != task {
			return nil
		}
		p, readErr := ReadProposal(path)
		if readErr != nil {
			return readErr
		}
		out = append(out, p)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ListProposals: walk: %w", err)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// ReadProposal parses a proposal file at the given absolute path.
func ReadProposal(path string) (Proposal, error) {
	f, err := os.Open(path)
	if err != nil {
		return Proposal{}, fmt.Errorf("ReadProposal: open: %w", err)
	}
	defer f.Close()
	fm, body, err := Parse(f)
	if err != nil {
		return Proposal{}, fmt.Errorf("ReadProposal: %s: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return Proposal{}, fmt.Errorf("ReadProposal: stat: %w", err)
	}

	persona, task := "", ""
	dir := filepath.Dir(path)
	parent := filepath.Dir(dir)
	grand := filepath.Dir(parent)
	if filepath.Base(grand) == "proposals" {
		persona = filepath.Base(parent)
		task = filepath.Base(dir)
	}

	slug := strings.TrimSuffix(filepath.Base(path), ".md")
	return Proposal{
		Persona: persona,
		Task:    task,
		Slug:    slug,
		FM:      fm,
		Body:    body,
		Path:    path,
		Created: info.ModTime(),
	}, nil
}
