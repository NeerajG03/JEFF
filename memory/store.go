// store.go — canonical-memory access: read API (FND) + write helpers (C).
// Only marlowe (via `jeff memory curate` / `jeff memory add`) should call the
// write helpers. The single-writer invariant is enforced at the CLI layer via
// JEFF_MEMORY_CAN_ADD=1; these helpers do not re-check it.
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Entry is one canonical memory entry resolved from disk.
type Entry struct {
	Scope  string // persona:<x> | repo:<y> | project:<z> | orchestrator
	Bucket Bucket
	Slug   string
	FM     CanonicalFrontmatter
	Body   string
	Path   string
}

// EntryFilter narrows ListEntries results. Empty fields mean "no filter".
type EntryFilter struct {
	Persona string
	Repo    string
	Project string
	Bucket  Bucket
	Status  string // accepted | superseded | ""
}

// ---- Read API (FND) ----

// ListEntries walks JEFF_HOME/memory/** and returns all canonical entries
// matching the filter, sorted by (scope, bucket, slug).
func ListEntries(jeffHome string, filter EntryFilter) ([]Entry, error) {
	root := MemoryRoot(jeffHome)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}

	var out []Entry

	scopes, err := collectScopes(jeffHome, filter)
	if err != nil {
		return nil, err
	}

	for _, sc := range scopes {
		for _, bucket := range Buckets {
			if filter.Bucket != "" && filter.Bucket != bucket {
				continue
			}
			files, err := bucketFiles(sc.path, bucket)
			if err != nil {
				return nil, err
			}
			for _, fp := range files {
				e, err := ReadEntry(fp)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: skipping unreadable memory entry %s: %v\n", fp, err)
					continue
				}
				e.Scope = sc.label
				e.Bucket = bucket
				if filter.Status != "" && e.FM.Status != filter.Status {
					continue
				}
				out = append(out, e)
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Scope != out[j].Scope {
			return out[i].Scope < out[j].Scope
		}
		if out[i].Bucket != out[j].Bucket {
			return out[i].Bucket < out[j].Bucket
		}
		return out[i].Slug < out[j].Slug
	})

	return out, nil
}

// ReadEntry parses a canonical memory file. Scope and Bucket are not set —
// callers that found the entry via ListEntries already know them; standalone
// callers can derive them from the file's path.
func ReadEntry(path string) (Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return Entry{}, fmt.Errorf("ReadEntry: open: %w", err)
	}
	defer f.Close()
	fm, body, err := ParseCanonical(f)
	if err != nil {
		return Entry{}, fmt.Errorf("ReadEntry: %s: %w", path, err)
	}
	slug := strings.TrimSuffix(filepath.Base(path), ".md")
	return Entry{
		Slug: slug,
		FM:   fm,
		Body: body,
		Path: path,
	}, nil
}

// ListScope returns canonical entries for a single, exact scope label
// (e.g. "persona:jenko", "repo:frontend", or "orchestrator"). Unlike
// ListEntries, it never broadens to sibling scopes — callers building a
// per-scope index (persona + N repos + orchestrator) call this once per
// scope instead of relying on EntryFilter's all-or-nothing semantics.
// status filters by FM.Status ("accepted", "superseded"); "" means no filter.
func ListScope(jeffHome, scopeLabel, status string) ([]Entry, error) {
	sPath, err := resolveScopePath(jeffHome, scopeLabel)
	if err != nil {
		return nil, fmt.Errorf("ListScope: %w", err)
	}
	if _, err := os.Stat(sPath); os.IsNotExist(err) {
		return nil, nil
	}

	var out []Entry
	for _, bucket := range Buckets {
		files, err := bucketFiles(sPath, bucket)
		if err != nil {
			return nil, err
		}
		for _, fp := range files {
			e, err := ReadEntry(fp)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: skipping unreadable memory entry %s: %v\n", fp, err)
				continue
			}
			e.Scope = scopeLabel
			e.Bucket = bucket
			if status != "" && e.FM.Status != status {
				continue
			}
			out = append(out, e)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Bucket != out[j].Bucket {
			return out[i].Bucket < out[j].Bucket
		}
		return out[i].Slug < out[j].Slug
	})

	return out, nil
}

// ---- Write helpers (C) ----

// WriteCanonical creates (or overwrites) a canonical memory entry on disk.
//
// scope is like "persona:jenko", "repo:gig", "project:foo", or "orchestrator".
// bucket is one of "core", "procedural", "semantic", "episodic".
// fm.Name is used as the file slug. fm.Scope and fm.ValidFrom are set from
// arguments if not already populated.
//
// This is the single write path for canonical entries. Only marlowe (running
// with JEFF_MEMORY_CAN_ADD=1) should call this.
func WriteCanonical(home, scope, bucket string, fm CanonicalFrontmatter, body string) (Entry, error) {
	if fm.Name == "" {
		return Entry{}, fmt.Errorf("WriteCanonical: fm.Name is required")
	}
	if !fm.Type.Valid() {
		return Entry{}, fmt.Errorf("WriteCanonical: invalid type %q", fm.Type)
	}

	if fm.Scope == "" {
		fm.Scope = scope
	}
	if fm.ValidFrom.IsZero() {
		fm.ValidFrom = time.Now().UTC()
	}
	if fm.Status == "" {
		fm.Status = "accepted"
	}

	sPath, err := resolveScopePath(home, scope)
	if err != nil {
		return Entry{}, fmt.Errorf("WriteCanonical: %w", err)
	}

	b := Bucket(bucket)
	var fp, dir string
	if b == BucketCore {
		dir = sPath
		fp = filepath.Join(sPath, "core.md")
	} else {
		dir = BucketPath(sPath, b)
		fp = filepath.Join(dir, fm.Name+".md")
		// Refuse silent overwrite for non-core entries.
		if _, err := os.Stat(fp); err == nil {
			return Entry{}, fmt.Errorf("memory entry %q already exists in %s/%s — use 'jeff memory add --supersede %s' to replace it", fm.Name, scope, bucket, fm.Name)
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Entry{}, fmt.Errorf("WriteCanonical: mkdir: %w", err)
	}

	f, err := os.Create(fp)
	if err != nil {
		return Entry{}, fmt.Errorf("WriteCanonical: create: %w", err)
	}
	defer f.Close()

	if err := writeCanonical(f, fm, body); err != nil {
		return Entry{}, fmt.Errorf("WriteCanonical: write: %w", err)
	}

	return Entry{
		Scope:  scope,
		Bucket: b,
		Slug:   fm.Name,
		FM:     fm,
		Body:   body,
		Path:   fp,
	}, nil
}

// Invalidate sets valid_to and superseded_by on an existing canonical entry.
// The file is updated in-place; it is never deleted (audit trail preserved).
func Invalidate(entryPath string, supersededBy string, at time.Time) error {
	f, err := os.Open(entryPath)
	if err != nil {
		return fmt.Errorf("Invalidate: open: %w", err)
	}
	fm, body, err := ParseCanonical(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("Invalidate: parse %s: %w", entryPath, err)
	}

	fm.ValidTo = &at
	fm.Status = "superseded"
	if supersededBy != "" {
		fm.SupersededBy = supersededBy
	}

	out, err := os.Create(entryPath)
	if err != nil {
		return fmt.Errorf("Invalidate: rewrite: %w", err)
	}
	defer out.Close()

	if err := writeCanonical(out, fm, body); err != nil {
		return fmt.Errorf("Invalidate: write: %w", err)
	}
	return nil
}

// Supersede writes a new canonical entry and invalidates the old one.
// newFm.Supersedes is extended with the old entry's slug before writing.
// The operation is not atomic at the filesystem level, but creates new before
// invalidating old to minimise the inconsistency window.
func Supersede(home, oldEntryPath string, newFm CanonicalFrontmatter, newBody string) (Entry, error) {
	// Derive scope + bucket from old entry's path so new entry lands nearby.
	scope, bucket, err := deriveScopeBucket(home, oldEntryPath)
	if err != nil {
		return Entry{}, fmt.Errorf("Supersede: derive scope: %w", err)
	}

	oldSlug := strings.TrimSuffix(filepath.Base(oldEntryPath), ".md")

	// Record what we supersede.
	newFm.Supersedes = append(newFm.Supersedes, oldSlug)

	newEntry, err := WriteCanonical(home, scope, string(bucket), newFm, newBody)
	if err != nil {
		return Entry{}, fmt.Errorf("Supersede: write new: %w", err)
	}

	if err := Invalidate(oldEntryPath, newFm.Name, time.Now().UTC()); err != nil {
		return newEntry, fmt.Errorf("Supersede: invalidate old: %w", err)
	}

	// Regenerate index for the affected bucket.
	if err := UpdateIndex(home, scope, string(bucket)); err != nil {
		return newEntry, fmt.Errorf("Supersede: update index: %w", err)
	}

	return newEntry, nil
}

// UpdateIndex regenerates INDEX.md for a scope/bucket directory.
// For BucketCore there is nothing to index (single file). For other buckets,
// INDEX.md lists all entries as a markdown table.
func UpdateIndex(home, scope, bucket string) error {
	b := Bucket(bucket)
	if b == BucketCore {
		return nil
	}

	sPath, err := resolveScopePath(home, scope)
	if err != nil {
		return fmt.Errorf("UpdateIndex: %w", err)
	}

	dir := BucketPath(sPath, b)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}

	files, err := bucketFiles(sPath, b)
	if err != nil {
		return fmt.Errorf("UpdateIndex: list: %w", err)
	}

	var rows []string
	for _, fp := range files {
		e, err := ReadEntry(fp)
		if err != nil {
			continue
		}
		validTo := ""
		if e.FM.ValidTo != nil {
			validTo = e.FM.ValidTo.Format("2006-01-02")
		}
		rows = append(rows, fmt.Sprintf("| %s | %s | %s | %s | %s | %s |",
			e.FM.Name, e.FM.Type, e.FM.Description,
			e.FM.Status, e.FM.ValidFrom.Format("2006-01-02"), validTo))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# INDEX — %s/%s\n\n", scope, bucket)
	sb.WriteString("| Name | Type | Description | Status | Valid From | Valid To |\n")
	sb.WriteString("|------|------|-------------|--------|------------|----------|\n")
	for _, row := range rows {
		sb.WriteString(row)
		sb.WriteByte('\n')
	}

	return os.WriteFile(filepath.Join(dir, "INDEX.md"), []byte(sb.String()), 0o644)
}

// ---- internal ----

type scopeRef struct {
	label string // persona:jenko | repo:gig | project:foo | orchestrator
	path  string // absolute path to the scope dir
}

// resolveScopePath converts a scope label like "persona:jenko" to an absolute path.
func resolveScopePath(home, scope string) (string, error) {
	parts := strings.SplitN(scope, ":", 2)
	switch parts[0] {
	case "persona":
		if len(parts) != 2 || parts[1] == "" {
			return "", fmt.Errorf("bad persona scope: %q", scope)
		}
		return PersonaScopePath(home, parts[1]), nil
	case "repo":
		if len(parts) != 2 || parts[1] == "" {
			return "", fmt.Errorf("bad repo scope: %q", scope)
		}
		return RepoScopePath(home, parts[1]), nil
	case "project":
		if len(parts) != 2 || parts[1] == "" {
			return "", fmt.Errorf("bad project scope: %q", scope)
		}
		return ProjectScopePath(home, parts[1]), nil
	case "orchestrator":
		return OrchestratorScopePath(home), nil
	default:
		return "", fmt.Errorf("unknown scope kind %q (want persona|repo|project|orchestrator)", parts[0])
	}
}

// deriveScopeBucket walks backwards from an entry file path to infer scope + bucket.
// Used by Supersede so callers don't have to pass them explicitly.
func deriveScopeBucket(home, entryPath string) (scope string, bucket Bucket, err error) {
	memRoot := MemoryRoot(home)
	rel, relErr := filepath.Rel(memRoot, entryPath)
	if relErr != nil {
		return "", "", fmt.Errorf("entry %s not under memory root %s", entryPath, memRoot)
	}
	// rel is like: personas/jenko/procedural/slug.md
	//              repos/gig/semantic/slug.md
	//              orchestrator/semantic/slug.md
	//              personas/jenko/core.md
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("unexpected path depth: %s", rel)
	}

	switch parts[0] {
	case "personas":
		if len(parts) < 3 {
			return "", "", fmt.Errorf("unexpected personas path: %s", rel)
		}
		scope = "persona:" + parts[1]
		if parts[2] == "core.md" {
			bucket = BucketCore
		} else {
			bucket = Bucket(parts[2])
		}
	case "repos":
		if len(parts) < 3 {
			return "", "", fmt.Errorf("unexpected repos path: %s", rel)
		}
		scope = "repo:" + parts[1]
		if parts[2] == "core.md" {
			bucket = BucketCore
		} else {
			bucket = Bucket(parts[2])
		}
	case "projects":
		if len(parts) < 3 {
			return "", "", fmt.Errorf("unexpected projects path: %s", rel)
		}
		scope = "project:" + parts[1]
		if parts[2] == "core.md" {
			bucket = BucketCore
		} else {
			bucket = Bucket(parts[2])
		}
	case "orchestrator":
		scope = "orchestrator"
		if len(parts) >= 2 && parts[1] == "core.md" {
			bucket = BucketCore
		} else if len(parts) >= 2 {
			bucket = Bucket(parts[1])
		}
	default:
		return "", "", fmt.Errorf("unknown scope root %q in path %s", parts[0], rel)
	}
	return scope, bucket, nil
}

func collectScopes(jeffHome string, f EntryFilter) ([]scopeRef, error) {
	var refs []scopeRef

	add := func(kind, name, path string) {
		if _, err := os.Stat(path); err != nil {
			return
		}
		label := name
		if kind != "" {
			label = kind + ":" + name
		}
		refs = append(refs, scopeRef{label: label, path: path})
	}

	if f.Persona != "" {
		add("persona", f.Persona, PersonaScopePath(jeffHome, f.Persona))
	} else if f.Repo == "" && f.Project == "" {
		names, err := readChildren(filepath.Join(MemoryRoot(jeffHome), "personas"))
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			add("persona", n, PersonaScopePath(jeffHome, n))
		}
	}
	if f.Repo != "" {
		add("repo", f.Repo, RepoScopePath(jeffHome, f.Repo))
	} else if f.Persona == "" && f.Project == "" {
		names, err := readChildren(filepath.Join(MemoryRoot(jeffHome), "repos"))
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			add("repo", n, RepoScopePath(jeffHome, n))
		}
	}
	if f.Project != "" {
		add("project", f.Project, ProjectScopePath(jeffHome, f.Project))
	} else if f.Persona == "" && f.Repo == "" {
		names, err := readChildren(filepath.Join(MemoryRoot(jeffHome), "projects"))
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			add("project", n, ProjectScopePath(jeffHome, n))
		}
	}
	if f.Persona == "" && f.Repo == "" && f.Project == "" {
		add("", "orchestrator", OrchestratorScopePath(jeffHome))
	}
	return refs, nil
}

func readChildren(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("readChildren: %w", err)
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

func bucketFiles(scopePath string, bucket Bucket) ([]string, error) {
	if bucket == BucketCore {
		fp := filepath.Join(scopePath, "core.md")
		if _, err := os.Stat(fp); err == nil {
			return []string{fp}, nil
		}
		return nil, nil
	}
	dir := BucketPath(scopePath, bucket)
	var out []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".md") && !strings.EqualFold(filepath.Base(path), "INDEX.md") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
