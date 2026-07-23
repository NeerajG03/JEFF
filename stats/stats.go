package stats

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/gig"
)

type Options struct {
	Since   time.Time
	Persona string
	Repo    string
	Outcome string
}

type TaskStat struct {
	ID           string             `json:"id"`
	Title        string             `json:"title"`
	Persona      string             `json:"persona,omitempty"`
	Repos        []string           `json:"repos,omitempty"`
	Outcome      string             `json:"outcome,omitempty"`
	PRs          map[string]string  `json:"prs,omitempty"`
	SkillsLoaded []string           `json:"skills_loaded,omitempty"`
	MemoryLoaded []string           `json:"memory_loaded,omitempty"`
	Checkpoints  int                `json:"checkpoints"`
	ClaimedAt    *time.Time         `json:"claimed_at,omitempty"`
	ClosedAt     *time.Time         `json:"closed_at,omitempty"`
	CycleTime    time.Duration      `json:"cycle_time_ns,omitempty"`
}

type Report struct {
	Since     time.Time            `json:"since"`
	Tasks     []TaskStat           `json:"tasks"`
	ByPersona map[string]Aggregate `json:"by_persona"`
	ByRepo    map[string]Aggregate `json:"by_repo"`
	ByOutcome map[string]int       `json:"by_outcome"`
	MemoryUse MemoryUse            `json:"memory_use"`
}

type Aggregate struct {
	Tasks        int           `json:"tasks"`
	AvgCycleTime time.Duration `json:"avg_cycle_time_ns,omitempty"`
	CycleSamples int           `json:"cycle_samples"`
}

type MemoryUse struct {
	WithMemory int `json:"with_memory"`
	WithSkills int `json:"with_skills"`
	Total      int `json:"total"`
}

func Collect(store *gig.Store, opts Options) (*Report, error) {
	closed := gig.StatusClosed
	cancelled := gig.StatusCancelled

	closedTasks, err := store.List(gig.ListParams{Status: &closed})
	if err != nil {
		return nil, err
	}
	cancelledTasks, err := store.List(gig.ListParams{Status: &cancelled})
	if err != nil {
		return nil, err
	}

	candidates := append(closedTasks, cancelledTasks...)

	var withinWindow []*gig.Task
	for _, t := range candidates {
		if t.ClosedAt != nil && t.ClosedAt.After(opts.Since) {
			withinWindow = append(withinWindow, t)
		}
	}

	var tasks []TaskStat
	for _, t := range withinWindow {
		attrs, err := store.Attrs(t.ID)
		if err != nil {
			continue
		}
		attrMap := make(map[string]string, len(attrs))
		for _, a := range attrs {
			attrMap[a.Key] = a.Value
		}

		persona := attrMap[jeff.AttrPersona]
		outcome := attrMap[jeff.AttrOutcome]

		var repos []string
		if raw, ok := attrMap[jeff.AttrRepos]; ok {
			json.Unmarshal([]byte(raw), &repos)
		}

		var prs map[string]string
		if raw, ok := attrMap[jeff.AttrPRURLs]; ok {
			json.Unmarshal([]byte(raw), &prs)
		}

		var skillsLoaded []string
		if raw, ok := attrMap[jeff.AttrSkillsLoaded]; ok {
			json.Unmarshal([]byte(raw), &skillsLoaded)
		}

		var memoryLoaded []string
		if raw, ok := attrMap[jeff.AttrMemoryLoaded]; ok {
			json.Unmarshal([]byte(raw), &memoryLoaded)
		}

		checkpoints, err := store.ListCheckpoints(t.ID)
		cpCount := 0
		if err == nil {
			cpCount = len(checkpoints)
		}

		events, err := store.Events(t.ID)
		var claimedAt *time.Time
		if err == nil {
			for _, e := range events {
				if e.Type == gig.EventStatusChanged && e.NewValue == string(gig.StatusInProgress) {
					v := e.Timestamp
					claimedAt = &v
					break
				}
			}
		}

		var cycleTime time.Duration
		if claimedAt != nil && t.ClosedAt != nil && t.ClosedAt.After(*claimedAt) {
			cycleTime = t.ClosedAt.Sub(*claimedAt)
		}

		ts := TaskStat{
			ID:           t.ID,
			Title:        t.Title,
			Persona:      persona,
			Repos:        repos,
			Outcome:      outcome,
			PRs:          prs,
			SkillsLoaded: skillsLoaded,
			MemoryLoaded: memoryLoaded,
			Checkpoints:  cpCount,
			ClaimedAt:    claimedAt,
			ClosedAt:     t.ClosedAt,
			CycleTime:    cycleTime,
		}
		tasks = append(tasks, ts)
	}

	filtered := applyFilters(tasks, opts)
	report := buildReport(filtered, opts.Since)
	return report, nil
}

func applyFilters(tasks []TaskStat, opts Options) []TaskStat {
	var out []TaskStat
	for _, t := range tasks {
		if opts.Persona != "" && t.Persona != opts.Persona {
			continue
		}
		if opts.Outcome != "" && t.Outcome != opts.Outcome {
			continue
		}
		if opts.Repo != "" {
			found := false
			for _, r := range t.Repos {
				if r == opts.Repo {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

func buildReport(tasks []TaskStat, since time.Time) *Report {
	byPersona := map[string]Aggregate{}
	byRepo := map[string]Aggregate{}
	byOutcome := map[string]int{}
	mem := MemoryUse{}

	for _, t := range tasks {
		p := t.Persona
		if p == "" {
			p = "(none)"
		}
		a := byPersona[p]
		a.Tasks++
		if t.CycleTime > 0 {
			a.AvgCycleTime += t.CycleTime
			a.CycleSamples++
		}
		byPersona[p] = a

		for _, r := range t.Repos {
			ra := byRepo[r]
			ra.Tasks++
			if t.CycleTime > 0 {
				ra.AvgCycleTime += t.CycleTime
				ra.CycleSamples++
			}
			byRepo[r] = ra
		}

		outcome := t.Outcome
		if outcome == "" {
			outcome = "(none)"
		}
		byOutcome[outcome]++

		mem.Total++
		if len(t.MemoryLoaded) > 0 {
			mem.WithMemory++
		}
		if len(t.SkillsLoaded) > 0 {
			mem.WithSkills++
		}
	}

	for k, a := range byPersona {
		if a.CycleSamples > 0 {
			a.AvgCycleTime = time.Duration(int64(a.AvgCycleTime) / int64(a.CycleSamples))
		}
		byPersona[k] = a
	}
	for k, a := range byRepo {
		if a.CycleSamples > 0 {
			a.AvgCycleTime = time.Duration(int64(a.AvgCycleTime) / int64(a.CycleSamples))
		}
		byRepo[k] = a
	}

	return &Report{
		Since:     since,
		Tasks:     tasks,
		ByPersona: byPersona,
		ByRepo:    byRepo,
		ByOutcome: byOutcome,
		MemoryUse: mem,
	}
}

func ParseSince(s string) (time.Time, error) {
	if len(s) < 2 || s[len(s)-1] != 'd' {
		return time.Time{}, errBadSince
	}
	days := 0
	for _, c := range s[:len(s)-1] {
		if c < '0' || c > '9' {
			return time.Time{}, errBadSince
		}
		days = days*10 + int(c-'0')
	}
	if days < 0 {
		return time.Time{}, errBadSince
	}
	return time.Now().Add(-time.Duration(days) * 24 * time.Hour), nil
}

var errBadSince = parseError{}

type parseError struct{}

func (parseError) Error() string {
	return "--since accepts Nd, e.g. 14d"
}

type SortableGroup struct {
	Name  string
	Value Aggregate
}

func SortGroups(m map[string]Aggregate) []SortableGroup {
	var out []SortableGroup
	for k, v := range m {
		out = append(out, SortableGroup{Name: k, Value: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Value.Tasks != out[j].Value.Tasks {
			return out[i].Value.Tasks > out[j].Value.Tasks
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func fmtDur(d time.Duration) string {
	h := d.Hours()
	if h < 10 {
		return round1(h) + "h"
	}
	return round1(h/24) + "d"
}

func round1(v float64) string {
	v = float64(int(v*10)) / 10
	return formatFloat(v)
}

func formatFloat(v float64) string {
	i := int(v)
	d := int((v - float64(i)) * 10)
	if d < 0 {
		d = 0
	}
	return itoa(i) + "." + itoa(d)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
