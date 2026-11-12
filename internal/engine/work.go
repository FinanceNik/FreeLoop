package engine

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"log"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/store"
)

// What a unit of work costs over time (GUI-051): each commit a session made, with
// the usage since its session's commit before, and what each percent of an
// account's limit bought. Models and subscriptions change under the user; these
// say whether the same work came to cost more. Costs are at today's prices, so a
// vendor's price change shows as none, and the tokens beside them do not move with
// prices at all.

// workBudget bounds the folding one report does, as activityBudget does.
const workBudget = 1500 * time.Millisecond

var workMu sync.Mutex

// WorkCommitCost is one commit and what it cost: its share of the usage since its
// session's commit before, split evenly among the commits that landed together.
type WorkCommitCost struct {
	SHA         string    `json:"sha"`
	SessionID   string    `json:"sessionId"`
	RootID      string    `json:"rootId"`
	Title       string    `json:"title"`
	ProjectID   string    `json:"projectId"`
	ProjectRoot string    `json:"projectRoot"`
	HarnessID   string    `json:"harnessId"`
	At          time.Time `json:"at"`
	Week        string    `json:"week"`
	// Model is the one that cost the most of it, and ProviderID its provider.
	Model      string  `json:"model"`
	ProviderID string  `json:"providerId"`
	Cost       float64 `json:"cost"`
	Unpriced   bool    `json:"unpriced"`
	Input      int64   `json:"input"`
	Output     int64   `json:"output"`
	CacheRead  int64   `json:"cacheRead"`
	CacheWrite int64   `json:"cacheWrite"`
	Added      int     `json:"added"`
	Deleted    int     `json:"deleted"`
	Files      int     `json:"files"`
	LandedWith int     `json:"landedWith"`
}

// WorkSpend is a week's usage of one model in one project by one agent, and whether
// a commit of its session followed it.
type WorkSpend struct {
	Week        string  `json:"week"`
	ProjectID   string  `json:"projectId"`
	ProjectRoot string  `json:"projectRoot"`
	HarnessID   string  `json:"harnessId"`
	ProviderID  string  `json:"providerId"`
	Model       string  `json:"model"`
	Committed   bool    `json:"committed"`
	Cost        float64 `json:"cost"`
	Tokens      int64   `json:"tokens"`
}

// LimitValue is what a week's usage of one limit bought: its spend at today's prices
// across the runs counted, and the percent of the limit those runs took.
type LimitValue struct {
	Week    string  `json:"week"`
	Source  string  `json:"source"`
	Window  string  `json:"window"`
	Label   string  `json:"label"`
	Spend   float64 `json:"spend"`
	Percent float64 `json:"percent"`
	Runs    int     `json:"runs"`
}

// WorkReport is the commits of the weeks asked for, the spend around them, and the
// value of each limit. Pending counts sessions not folded yet; asking again
// continues. Unattributed counts commits no session made, as one made in a
// terminal.
type WorkReport struct {
	Since        string           `json:"since"`
	Commits      []WorkCommitCost `json:"commits"`
	Spend        []WorkSpend      `json:"spend"`
	Limits       []LimitValue     `json:"limits"`
	Unattributed int              `json:"unattributed"`
	Pending      int              `json:"pending"`
}

// Limit runs count only when they moved this much: reports come in whole percent,
// so a smaller step is mostly rounding.
const (
	minLimitStep    = 10.0
	minLimitReports = 3
)

// weekOf is the Monday a time's week starts on, in local time.
func weekOf(t time.Time) string {
	t = t.Local()
	day := (int(t.Weekday()) + 6) % 7
	return time.Date(t.Year(), t.Month(), t.Day()-day, 0, 0, 0, 0, time.Local).Format(time.DateOnly)
}

// foldWork folds what moved on, within the budget.
func (e *Engine) foldWork(ctx context.Context) int {
	workMu.Lock()
	defer workMu.Unlock()
	pending, err := e.store.FoldWork(ctx, time.Now().Add(workBudget), e.resolveCommits)
	if err != nil {
		log.Printf("engine: work: %v", err)
	}
	return pending
}

// resolveCommits finds, for a transcript from before commits were listed, the
// commits a session's HEAD moved over. A session whose folder is gone cannot say.
func (e *Engine) resolveCommits(ctx context.Context, sid, old, now string) ([]event.CommitInfo, bool) {
	env, ses, err := e.sessionGit(ctx, sid)
	if err != nil {
		return nil, false
	}
	return newCommits(ctx, env, old, now, e.trusted(ses.ProjectID)), true
}

// WorkCost returns the commits and spend of the last weeks weeks, this one included.
func (e *Engine) WorkCost(ctx context.Context, weeks int) (WorkReport, error) {
	if weeks <= 0 || weeks > 104 {
		weeks = 26
	}
	pending := e.foldWork(ctx)
	start, _ := time.ParseInLocation(time.DateOnly, weekOf(time.Now()), time.Local)
	rep, err := e.workCost(ctx, start.AddDate(0, 0, -7*(weeks-1)))
	rep.Pending = pending
	return rep, err
}

// workCost reports the commits from since on, from what is folded.
func (e *Engine) workCost(ctx context.Context, since time.Time) (WorkReport, error) {
	rep := WorkReport{Since: since.Format(time.DateOnly), Commits: []WorkCommitCost{}, Spend: []WorkSpend{}, Limits: []LimitValue{}}
	commits, usage, unattributed, err := e.store.WorkSince(ctx, since)
	if err != nil {
		return WorkReport{}, err
	}
	rep.Unattributed = unattributed
	prices := config.EffectivePrices(e.Config())
	price := func(u store.WorkUsage) (float64, bool) {
		p, ok := config.FindPrice(prices, u.ProviderID, u.Model)
		if !ok {
			return 0, false
		}
		return p.Cost(u.Input, u.Output, u.CacheRead, u.CacheWrite, u.CacheWrite1h), true
	}
	sessions := map[string]WorkCommitCost{}
	about := func(id string) WorkCommitCost {
		if s, ok := sessions[id]; ok {
			return s
		}
		var s WorkCommitCost
		if ses, err := e.store.GetSession(ctx, id); err == nil {
			s = WorkCommitCost{Title: ses.Title, ProjectID: ses.ProjectID, ProjectRoot: ses.ProjectRoot, HarnessID: ses.HarnessID}
		}
		sessions[id] = s
		return s
	}

	// Each commit's unit is the turn it landed in. Usage goes to the first unit after
	// it among its root's: by the order of events within one session, and by time
	// between a session and its subagents'.
	type unit struct {
		session string
		seq     int64
		ts      time.Time
		commits []int
	}
	units := map[string][]*unit{}
	for i, c := range commits {
		list := units[c.RootID]
		if n := len(list); n > 0 && list[n-1].session == c.SessionID && list[n-1].seq == c.UnitSeq {
			list[n-1].commits = append(list[n-1].commits, i)
			continue
		}
		units[c.RootID] = append(list, &unit{session: c.SessionID, seq: c.UnitSeq, ts: c.UnitTS, commits: []int{i}})
	}
	after := func(un *unit, u store.WorkUsage) bool {
		if un.session == u.SessionID {
			return un.seq >= u.EndSeq
		}
		return !un.ts.Before(u.TS)
	}
	type acc struct {
		cost                                 float64
		unpriced                             bool
		input, output, cacheRead, cacheWrite int64
		// byModel weighs each model by its cost; tokens break a tie between
		// unpriced ones.
		byModel map[[2]string]float64
	}
	perUnit := map[*unit]*acc{}
	spend := map[WorkSpend]*WorkSpend{}
	for _, u := range usage {
		var to *unit
		for _, un := range units[u.RootID] {
			if after(un, u) {
				to = un
				break
			}
		}
		cost, priced := price(u)
		if to != nil {
			a := perUnit[to]
			if a == nil {
				a = &acc{byModel: map[[2]string]float64{}}
				perUnit[to] = a
			}
			a.cost += cost
			a.unpriced = a.unpriced || !priced
			a.input += u.Input
			a.output += u.Output
			a.cacheRead += u.CacheRead
			a.cacheWrite += u.CacheWrite
			a.byModel[[2]string{u.ProviderID, u.Model}] += cost + 1e-12*float64(u.Input+u.Output)
		}
		if u.TS.Before(since) {
			continue
		}
		info := about(u.SessionID)
		k := WorkSpend{Week: weekOf(u.TS), ProjectID: u.ProjectID, ProjectRoot: info.ProjectRoot, HarnessID: u.HarnessID, ProviderID: u.ProviderID, Model: u.Model, Committed: to != nil}
		s := spend[k]
		if s == nil {
			s = &k
			spend[k] = s
		}
		s.Cost += cost
		s.Tokens += u.Input + u.Output + u.CacheRead + u.CacheWrite
	}
	for root, list := range units {
		for _, un := range list {
			a := perUnit[un]
			n := len(un.commits)
			for _, i := range un.commits {
				c := commits[i]
				if c.Before {
					// From before the window: it only said where the next one starts.
					continue
				}
				info := about(c.SessionID)
				if info.Title == "" {
					info = about(root)
				}
				w := WorkCommitCost{SHA: c.SHA, SessionID: c.SessionID, RootID: root, Title: info.Title, ProjectID: c.ProjectID, ProjectRoot: info.ProjectRoot,
					HarnessID: info.HarnessID, At: c.UnitTS, Week: weekOf(c.UnitTS), Added: c.Added, Deleted: c.Deleted, Files: c.Files, LandedWith: n}
				if a != nil {
					share := 1 / float64(n)
					w.Cost, w.Unpriced = a.cost*share, a.unpriced
					w.Input, w.Output = int64(float64(a.input)*share), int64(float64(a.output)*share)
					w.CacheRead, w.CacheWrite = int64(float64(a.cacheRead)*share), int64(float64(a.cacheWrite)*share)
					best := -1.0
					for m, v := range a.byModel {
						if v > best || (v == best && m[1] < w.Model) {
							w.ProviderID, w.Model, best = m[0], m[1], v
						}
					}
				}
				rep.Commits = append(rep.Commits, w)
			}
		}
	}
	sort.Slice(rep.Commits, func(i, j int) bool { return rep.Commits[i].At.Before(rep.Commits[j].At) })
	for _, s := range spend {
		rep.Spend = append(rep.Spend, *s)
	}
	sort.Slice(rep.Spend, func(i, j int) bool {
		a, b := rep.Spend[i], rep.Spend[j]
		if a.Week != b.Week {
			return a.Week < b.Week
		}
		return a.ProjectRoot+a.HarnessID+a.ProviderID+a.Model < b.ProjectRoot+b.HarnessID+b.ProviderID+b.Model
	})
	limits, err := e.limitValues(ctx, since, price)
	if err != nil {
		return WorkReport{}, err
	}
	rep.Limits = limits
	return rep, nil
}

// limitValues adds up, per week and limit, the spend and percent of the runs that
// moved enough: from a run's first report to its highest, the usage of the agents
// that reported it. Use outside the app takes from the same limit unseen.
func (e *Engine) limitValues(ctx context.Context, since time.Time, price func(store.WorkUsage) (float64, bool)) ([]LimitValue, error) {
	spans, err := e.store.LimitSpans(ctx, since)
	if err != nil {
		return nil, err
	}
	type key struct{ week, source, window string }
	acc := map[key]*LimitValue{}
	var order []key
	for _, sp := range spans {
		step := sp.Peak - sp.FirstUse
		if step < minLimitStep || sp.Reports < minLimitReports || sp.PeakAt.Before(since) {
			continue
		}
		rows, err := e.store.BucketUsage(ctx, sp.First, sp.PeakAt, sp.Harnesses)
		if err != nil {
			return nil, err
		}
		var cost float64
		for _, u := range rows {
			c, _ := price(u)
			cost += c
		}
		if cost == 0 {
			continue
		}
		k := key{weekOf(sp.PeakAt), sp.Source, sp.Window}
		v := acc[k]
		if v == nil {
			v = &LimitValue{Week: k.week, Source: sp.Source, Window: sp.Window, Label: sp.Label}
			acc[k] = v
			order = append(order, k)
		}
		v.Spend += cost
		v.Percent += step
		v.Runs++
	}
	out := []LimitValue{}
	for _, k := range order {
		out = append(out, *acc[k])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Week != out[j].Week {
			return out[i].Week < out[j].Week
		}
		return out[i].Source+out[i].Window < out[j].Source+out[j].Window
	})
	return out, nil
}

// ExportWorkCost writes the commits of the last weeks weeks and what each cost to a
// CSV file at path.
func (e *Engine) ExportWorkCost(ctx context.Context, weeks int, path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("the export needs an absolute path")
	}
	rep, err := e.WorkCost(ctx, weeks)
	if err != nil {
		return err
	}
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.Write([]string{"week", "committed", "commit", "session", "title", "project", "agent", "provider", "model", "cost_usd", "input", "output", "cache_read", "cache_write", "lines_added", "lines_deleted", "files", "landed_with"})
	n := func(v int64) string { return strconv.FormatInt(v, 10) }
	for _, c := range rep.Commits {
		cost := ""
		if !c.Unpriced {
			cost = strconv.FormatFloat(c.Cost, 'f', 6, 64)
		}
		w.Write([]string{c.Week, c.At.Format(time.RFC3339), c.SHA, c.SessionID, cell(c.Title), cell(c.ProjectRoot), c.HarnessID, cell(c.ProviderID), cell(c.Model), cost,
			n(c.Input), n(c.Output), n(c.CacheRead), n(c.CacheWrite), strconv.Itoa(c.Added), strconv.Itoa(c.Deleted), strconv.Itoa(c.Files), strconv.Itoa(c.LandedWith)})
	}
	w.Flush()
	env, err := e.localEnv(filepath.Dir(path), nil)
	if err != nil {
		return err
	}
	return env.FS().WriteFile(path, b.Bytes())
}
