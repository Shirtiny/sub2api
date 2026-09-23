package service

import (
	_ "embed"
	"html"
	"strings"
)

//go:embed templates/waitlist_confirmation.html
var waitlistConfirmationTemplate string

func waitlistConfirmationHTML(siteName string) string {
	siteName = strings.TrimSpace(siteName)
	if siteName == "" {
		siteName = "Sub2API"
	}
	// Both placeholders occur only in HTML text nodes. Replace in one pass so
	// even a site name containing a placeholder remains literal, escaped text.
	return strings.NewReplacer(
		"{{SITE_NAME}}", html.EscapeString(siteName),
		"{{MESSAGE}}", html.EscapeString(waitlistConfirmationMessage),
	).Replace(waitlistConfirmationTemplate)
}
