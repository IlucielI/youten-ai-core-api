package templates

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	texttemplate "text/template"
)

var (
	parsedHTMLEmails = template.Must(template.ParseFS(emailFS, "emails/*.html"))
	parsedTextEmails = texttemplate.Must(texttemplate.ParseFS(emailFS, "emails/*.txt"))
)

// AdminResetPasswordEmailData holds the template context for administrative password reset emails.
type AdminResetPasswordEmailData struct {
	FullName string
	Token    string
}

// RenderAdminResetPasswordEmail renders HTML and plain text bodies for admin password reset emails.
func RenderAdminResetPasswordEmail(fullName, token string) (htmlBody, textBody string, err error) {
	data := AdminResetPasswordEmailData{
		FullName: strings.TrimSpace(fullName),
		Token:    strings.TrimSpace(token),
	}

	var htmlBuf bytes.Buffer
	if err := parsedHTMLEmails.ExecuteTemplate(&htmlBuf, "admin_password_reset.html", data); err != nil {
		return "", "", fmt.Errorf("execute admin_password_reset.html: %w", err)
	}

	var textBuf bytes.Buffer
	if err := parsedTextEmails.ExecuteTemplate(&textBuf, "admin_password_reset.txt", data); err != nil {
		return "", "", fmt.Errorf("execute admin_password_reset.txt: %w", err)
	}

	return htmlBuf.String(), textBuf.String(), nil
}
