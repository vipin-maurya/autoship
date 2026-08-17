package notes

import (
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/vipinm/autoship/internal/release"
)

// BuildResult is what the build stages observed, and is everything the
// technical notes need that the release itself does not carry. Unlike customer
// copy, this can be fully generated (spec §7.1).
type BuildResult struct {
	SHA         string
	CommitRange string
	Commits     []string
	Tests       string
	Lint        string
	UIValidated string
	BundlePath  string
	BundleBytes int64
	Track       string
	At          time.Time
}

const technicalTemplate = `# Release Notes (Technical) — v{{.Rel.Name}}

- **versionName:** {{.Rel.Name}}
- **versionCode:** {{.Rel.Code}}
- **Release kind:** {{.Rel.Kind}}
- **Previous release:** {{if .Rel.PreviousName}}{{.Rel.PreviousName}} ({{.Rel.PreviousCode}}){{else}}none{{end}}
- **Commit:** {{.Res.SHA}}
- **Commit range:** {{.Res.CommitRange}}
- **Track:** {{.Res.Track}}
- **Built:** {{.Built}}

## Verification

- **Unit tests:** {{.Res.Tests}}
- **Lint:** {{.Res.Lint}}
- **UI validation:** {{.Res.UIValidated}}

## Artefact

- **Bundle:** {{.Res.BundlePath}}
{{- if .Res.BundleBytes}}
- **Size:** {{.SizeMB}} MB
{{- end}}

## Changes
{{range .Res.Commits}}
- {{.}}
{{- else}}
- (no commit subjects recorded)
{{- end}}
`

// Technical renders the technical release notes kept alongside the artefacts.
func Technical(rel release.Release, res BuildResult) string {
	at := res.At
	if at.IsZero() {
		at = time.Now()
	}
	data := struct {
		Rel    release.Release
		Res    BuildResult
		Built  string
		SizeMB string
	}{
		Rel:    rel,
		Res:    res,
		Built:  at.UTC().Format(time.RFC3339),
		SizeMB: fmt.Sprintf("%.1f", float64(res.BundleBytes)/(1024*1024)),
	}

	tmpl := template.Must(template.New("technical").Parse(technicalTemplate))
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		// The template is a constant and the data is a plain struct, so this
		// cannot fail in practice; surface it rather than hiding it if it does.
		return fmt.Sprintf("technical notes could not be rendered: %v", err)
	}
	return sb.String()
}
