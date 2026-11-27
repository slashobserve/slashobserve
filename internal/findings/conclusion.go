package findings

import (
	"fmt"
	"strings"
)

const (
	headingCauses      = "### Causes"
	headingUnconfirmed = "### Unconfirmed causes"
)

var markdownEscaper = strings.NewReplacer(`\`, `\\`, "*", `\*`, "_", `\_`, "`", "\\`", "[", `\[`, "]", `\]`, "<", `\<`, ">", `\>`, "#", `\#`, "|", `\|`)

func WithCauses(markdown string, causes []Cause) string {
	section := CausesMarkdown(causes)
	if section == "" {
		return markdown
	}
	return markdown + "\n\n" + section
}

func CausesMarkdown(causes []Cause) string {
	var stated, unconfirmed []string
	for _, c := range causes {
		line := causeLine(c, causes)
		if c.Stated {
			stated = append(stated, fmt.Sprintf("%d. %s", len(stated)+1, line))
		} else {
			unconfirmed = append(unconfirmed, "- "+line)
		}
	}
	var parts []string
	if len(stated) > 0 {
		parts = append(parts, headingCauses+"\n\n"+strings.Join(stated, "\n"))
	}
	if len(unconfirmed) > 0 {
		parts = append(parts, headingUnconfirmed+"\n\n"+strings.Join(unconfirmed, "\n"))
	}
	return strings.Join(parts, "\n\n")
}

func causeLine(c Cause, all []Cause) string {
	head := []string{"**" + escape(c.Component.Text()) + "**"}
	if c.Component.Kind != "" && c.Component.Kind != "other" {
		head = append(head, c.Component.Kind)
	}
	if c.KnockOnOf != nil && *c.KnockOnOf < len(all) {
		head = append(head, "knock-on of "+escape(all[*c.KnockOnOf].Component.Text()))
	}
	switch {
	case c.Band != "" && c.Score != nil:
		head = append(head, fmt.Sprintf("%s confidence (%.2f)", c.Band, *c.Score))
	case c.Estimate != "":
		head = append(head, "estimated "+c.Estimate+", not scored")
	}
	parts := []string{strings.Join(head, " · ") + ": " + sentenceOf(escape(c.Mechanism))}
	if t := c.Trigger; t != nil {
		trigger := "Trigger: " + strings.ReplaceAll(t.Kind, "_", " ") + " " + escape(t.Key)
		if t.Value != "" {
			trigger += " = " + escape(t.Value)
		}
		parts = append(parts, trigger+".")
	}
	if c.Onset != nil {
		parts = append(parts, "Started "+c.Onset.UTC().Format("2006-01-02 15:04 UTC")+".")
	}
	return strings.Join(parts, " ")
}

func escape(text string) string {
	return markdownEscaper.Replace(text)
}

func sentenceOf(text string) string {
	if strings.HasSuffix(text, ".") || strings.HasSuffix(text, "!") || strings.HasSuffix(text, "?") {
		return text
	}
	return text + "."
}
