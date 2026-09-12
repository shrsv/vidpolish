// Package ytmeta builds the tags/description that go on every YouTube
// upload, shared by the CLI and the local UI's job runner so both apply
// the same rules.
package ytmeta

import (
	"fmt"
	"strings"
	"text/template"
)

// MergeTags merges configured default tags with the fixed "vidpolish"
// tag, deduplicating while preserving order (fixed tag first).
func MergeTags(defaultTags []string) []string {
	tags := []string{"vidpolish"}
	seen := map[string]bool{"vidpolish": true}
	for _, t := range defaultTags {
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		tags = append(tags, t)
	}
	return tags
}

// BuildDescription renders tmplText with {{.Title}}, ensuring the literal
// word "vidpolish" always appears even if the template omits it or fails
// to render. warn, if non-nil, receives a message when the template is
// invalid (falling back to the plain title).
func BuildDescription(tmplText, title string, warn func(string)) string {
	rendered := renderTemplate(tmplText, title, warn)
	if !strings.Contains(rendered, "vidpolish") {
		if rendered != "" {
			rendered += "\n\n"
		}
		rendered += "vidpolish"
	}
	return rendered
}

func renderTemplate(tmplText, title string, warn func(string)) string {
	if tmplText == "" {
		return ""
	}
	tmpl, err := template.New("description").Parse(tmplText)
	if err != nil {
		if warn != nil {
			warn(fmt.Sprintf("invalid description_template, falling back to plain title: %v", err))
		}
		return title
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, struct{ Title string }{title}); err != nil {
		if warn != nil {
			warn(fmt.Sprintf("rendering description_template failed, falling back to plain title: %v", err))
		}
		return title
	}
	return buf.String()
}
