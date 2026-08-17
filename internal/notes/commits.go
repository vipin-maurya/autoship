package notes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/vipinm/autoship/internal/gitrepo"
	"github.com/vipinm/autoship/internal/release"
)

// DefaultCommitTemplate renders the subjects as a plain bulleted list. It is
// deliberately dull: this provider is a fallback, and copy generated from
// commit subjects is never as good as copy someone wrote.
const DefaultCommitTemplate = `What's new in {{.Version}}:
{{range .Items}}• {{.}}
{{end}}`

// housekeepingTypes are conventional-commit prefixes that say nothing to a
// tester and are dropped from customer-facing copy.
var housekeepingTypes = map[string]bool{
	"chore": true, "ci": true, "build": true, "test": true,
	"docs": true, "style": true, "refactor": true,
}

// CommitsProvider derives notes from conventional-commit subjects since the
// last release tag.
type CommitsProvider struct {
	RepoPath string
	// TemplatePath is optional; DefaultCommitTemplate is used when it is empty.
	TemplatePath string
	// Subjects overrides how commit subjects are obtained. Tests set it; the
	// zero value reads them from git.
	Subjects func(repoPath, revRange string) ([]string, error)
}

type commitData struct {
	Version string
	Code    int
	Items   []string
}

// Notes renders the customer-facing copy, or ErrNoNotes when nothing since the
// last tag is worth telling a tester about.
func (p CommitsProvider) Notes(_ context.Context, rel release.Release) (string, error) {
	subjects, err := p.subjects()
	if err != nil {
		return "", err
	}
	items := Highlights(subjects)
	if len(items) == 0 {
		return "", fmt.Errorf("%w: no user-facing commits since the last release", ErrNoNotes)
	}

	text, err := p.render(commitData{Version: rel.Name, Code: rel.Code, Items: items})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

func (p CommitsProvider) subjects() ([]string, error) {
	revRange, err := p.revRange()
	if err != nil {
		return nil, err
	}
	get := p.Subjects
	if get == nil {
		get = gitrepo.Subjects
	}
	return get(p.RepoPath, revRange)
}

func (p CommitsProvider) revRange() (string, error) {
	if p.Subjects != nil {
		// An injected source decides its own range.
		return "", nil
	}
	tag, err := gitrepo.LastTag(p.RepoPath)
	if err != nil {
		return "", fmt.Errorf("find the last release tag: %w", err)
	}
	if tag == "" {
		return "", nil
	}
	return tag + "..HEAD", nil
}

func (p CommitsProvider) render(data commitData) (string, error) {
	text := DefaultCommitTemplate
	if strings.TrimSpace(p.TemplatePath) != "" {
		path := p.TemplatePath
		if !filepath.IsAbs(path) && p.RepoPath != "" {
			path = filepath.Join(p.RepoPath, filepath.FromSlash(path))
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read notes template: %w", err)
		}
		text = string(raw)
	}
	tmpl, err := template.New("notes").Parse(text)
	if err != nil {
		return "", fmt.Errorf("parse notes template: %w", err)
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("render notes template: %w", err)
	}
	return sb.String(), nil
}

// Highlights keeps the commit subjects a tester would care about, with their
// conventional-commit type prefix removed.
func Highlights(subjects []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range subjects {
		typ, rest, ok := splitConventional(s)
		if ok && housekeepingTypes[typ] {
			continue
		}
		if !ok {
			rest = strings.TrimSpace(s)
		}
		if rest == "" || seen[rest] {
			continue
		}
		seen[rest] = true
		out = append(out, rest)
	}
	return out
}

// splitConventional splits "feat(ui)!: add settings screen" into "feat" and
// "add settings screen".
func splitConventional(subject string) (typ, rest string, ok bool) {
	colon := strings.Index(subject, ":")
	if colon < 0 {
		return "", subject, false
	}
	head := strings.TrimSpace(subject[:colon])
	head = strings.TrimSuffix(head, "!")
	if paren := strings.Index(head, "("); paren >= 0 {
		head = head[:paren]
	}
	head = strings.ToLower(strings.TrimSpace(head))
	if head == "" || strings.ContainsAny(head, " \t") {
		return "", subject, false
	}
	return head, strings.TrimSpace(subject[colon+1:]), true
}
