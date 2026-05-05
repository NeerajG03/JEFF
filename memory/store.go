// store.go — read-only canonical-memory access.
// Write/invalidate functions live in curate.go (Worker C). FND ships only the
// reads so Worker D can implement list/show without waiting on C.
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
					return nil, err
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

// ---- internal ----

type scopeRef struct {
	label string // persona:jenko | repo:gig | project:foo | orchestrator
	path  string // absolute path to the scope dir
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

	// Honor exact filters first; otherwise enumerate all.
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
