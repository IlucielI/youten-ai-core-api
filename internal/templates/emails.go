package templates

import (
	"bytes"
	"embed"
	"fmt"
	htmlTemplate "html/template"
	"strings"
	textTemplate "text/template"
)

//go:embed emails/*.html.tmpl emails/*.txt.tmpl
var emailFS embed.FS

var (
	parsedEmailHTML = htmlTemplate.Must(htmlTemplate.ParseFS(emailFS, "emails/*.html.tmpl"))
	parsedEmailText = textTemplate.Must(textTemplate.ParseFS(emailFS, "emails/*.txt.tmpl"))
)

// VerifyEmailData holds parameters for rendering account verification emails.
type VerifyEmailData struct {
	FullName  string
	Code      string
	VerifyURL string
}

// ResetPasswordEmailData holds parameters for rendering password reset emails.
type ResetPasswordEmailData struct {
	FullName string
	Token    string
	ResetURL string
}

// WaitlistEmailData holds parameters for rendering beta waitlist confirmation emails.
type WaitlistEmailData struct {
	Platform    string
	CompanySize string
	Status      string
}

// RenderVerifyEmail renders both HTML and plain text bodies for user registration verification.
func RenderVerifyEmail(data VerifyEmailData) (htmlBody, textBody string, err error) {
	var htmlBuf bytes.Buffer
	if err := parsedEmailHTML.ExecuteTemplate(&htmlBuf, "verify_email.html.tmpl", data); err != nil {
		return "", "", fmt.Errorf("execute verify_email.html.tmpl: %w", err)
	}

	var textBuf bytes.Buffer
	if err := parsedEmailText.ExecuteTemplate(&textBuf, "verify_email.txt.tmpl", data); err != nil {
		return "", "", fmt.Errorf("execute verify_email.txt.tmpl: %w", err)
	}

	return strings.TrimSpace(htmlBuf.String()), strings.TrimSpace(textBuf.String()), nil
}

// RenderResetPasswordEmail renders both HTML and plain text bodies for password recovery.
func RenderResetPasswordEmail(data ResetPasswordEmailData) (htmlBody, textBody string, err error) {
	var htmlBuf bytes.Buffer
	if err := parsedEmailHTML.ExecuteTemplate(&htmlBuf, "reset_password.html.tmpl", data); err != nil {
		return "", "", fmt.Errorf("execute reset_password.html.tmpl: %w", err)
	}

	var textBuf bytes.Buffer
	if err := parsedEmailText.ExecuteTemplate(&textBuf, "reset_password.txt.tmpl", data); err != nil {
		return "", "", fmt.Errorf("execute reset_password.txt.tmpl: %w", err)
	}

	return strings.TrimSpace(htmlBuf.String()), strings.TrimSpace(textBuf.String()), nil
}

// RenderWaitlistEmail renders both HTML and plain text bodies for waitlist confirmation.
func RenderWaitlistEmail(data WaitlistEmailData) (htmlBody, textBody string, err error) {
	var htmlBuf bytes.Buffer
	if err := parsedEmailHTML.ExecuteTemplate(&htmlBuf, "waitlist.html.tmpl", data); err != nil {
		return "", "", fmt.Errorf("execute waitlist.html.tmpl: %w", err)
	}

	var textBuf bytes.Buffer
	if err := parsedEmailText.ExecuteTemplate(&textBuf, "waitlist.txt.tmpl", data); err != nil {
		return "", "", fmt.Errorf("execute waitlist.txt.tmpl: %w", err)
	}

	return strings.TrimSpace(htmlBuf.String()), strings.TrimSpace(textBuf.String()), nil
}
