package dtos

// EmailMessage represents the content and recipients of an outgoing email.
type EmailMessage struct {
	To       []string `json:"to"`
	Cc       []string `json:"cc,omitempty"`
	Bcc      []string `json:"bcc,omitempty"`
	Subject  string   `json:"subject"`
	TextBody string   `json:"text_body,omitempty"`
	HTMLBody string   `json:"html_body,omitempty"`
	ReplyTo  string   `json:"reply_to,omitempty"`
}
