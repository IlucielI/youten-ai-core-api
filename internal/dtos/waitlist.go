package dtos

import (

)

// WaitlistRequest represents the input payload for joining the meeting voice bot beta waitlist.
type WaitlistRequest struct {
	Email       string `json:"email"`
	Platform    string `json:"platform"`
	CompanySize string `json:"company_size"`
}


// WaitlistResponse represents the response envelope confirming waitlist registration.
type WaitlistResponse struct {
	Email       string `json:"email"`
	Platform    string `json:"platform"`
	CompanySize string `json:"company_size"`
	Status      string `json:"status"`
	Message     string `json:"message"`
}
