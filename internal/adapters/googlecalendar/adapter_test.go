package googlecalendar_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"code-base-golang/internal/adapters/googlecalendar"
	"code-base-golang/internal/constants"
)

func TestExtractMeetingLink(t *testing.T) {
	tests := []struct {
		name         string
		confURI      string
		location     string
		desc         string
		wantURL      string
		wantPlatform string
	}{
		{
			name:         "Google Meet in conferenceData",
			confURI:      "https://meet.google.com/abc-defg-hij",
			location:     "",
			desc:         "",
			wantURL:      "https://meet.google.com/abc-defg-hij",
			wantPlatform: constants.MeetingPlatformGoogleMeet,
		},
		{
			name:         "Google Meet in location text",
			confURI:      "",
			location:     "Join here: https://meet.google.com/xyz-uvwx-rst Room A",
			desc:         "Meeting notes",
			wantURL:      "https://meet.google.com/xyz-uvwx-rst",
			wantPlatform: constants.MeetingPlatformGoogleMeet,
		},
		{
			name:         "Zoom in description text",
			confURI:      "",
			location:     "Online",
			desc:         "Meeting link: https://us02web.zoom.us/j/123456789?pwd=abcdef Please be on time.",
			wantURL:      "https://us02web.zoom.us/j/123456789?pwd=abcdef",
			wantPlatform: constants.MeetingPlatformZoom,
		},
		{
			name:         "MS Teams in description",
			confURI:      "",
			location:     "",
			desc:         `Click to join: https://teams.microsoft.com/l/meetup-join/19%3ameeting_xyz%40thread.v2/0?context={"Tid":"123"}`,
			wantURL:      `https://teams.microsoft.com/l/meetup-join/19%3ameeting_xyz%40thread.v2/0?context={"Tid":"123"}`,
			wantPlatform: constants.MeetingPlatformMSTeams,
		},
		{
			name:         "Offline meeting without links",
			confURI:      "",
			location:     "Meeting Room 3B, Floor 2",
			desc:         "Discuss annual strategy offline",
			wantURL:      "",
			wantPlatform: constants.MeetingPlatformNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotURL, gotPlatform := googlecalendar.ExtractMeetingLink(tc.confURI, tc.location, tc.desc)
			if gotURL != tc.wantURL {
				t.Errorf("ExtractMeetingLink() gotURL = %q, want %q", gotURL, tc.wantURL)
			}
			if gotPlatform != tc.wantPlatform {
				t.Errorf("ExtractMeetingLink() gotPlatform = %q, want %q", gotPlatform, tc.wantPlatform)
			}
		})
	}
}

func TestGetAuthURL(t *testing.T) {
	adapter := googlecalendar.NewAdapter(googlecalendar.Config{
		ClientID:    "test-client-id",
		RedirectURI: "https://youten.ai/callback",
	})

	url := adapter.GetAuthURL("csrf-state-1234")
	if !strings.Contains(url, "client_id=test-client-id") {
		t.Errorf("expected client_id in URL, got %s", url)
	}
	if !strings.Contains(url, "state=csrf-state-1234") {
		t.Errorf("expected state in URL, got %s", url)
	}
	if !strings.Contains(url, "calendar.events.readonly") {
		t.Errorf("expected readonly scope in URL, got %s", url)
	}
}

func TestAdapter_ExchangeCode_And_RefreshToken_Errors(t *testing.T) {
	adapter := googlecalendar.NewAdapter(googlecalendar.Config{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
	})

	ctx := context.Background()

	// Empty code error
	_, _, err := adapter.ExchangeCode(ctx, "")
	if err == nil {
		t.Error("expected error with empty code, got nil")
	}

	// Empty refresh token error
	_, err = adapter.RefreshToken(ctx, "")
	if err == nil {
		t.Error("expected error with empty refresh token, got nil")
	}
}

func TestAdapter_FetchEvents_Mock(t *testing.T) {
	// Mock server that returns a calendar event JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/primary/events") {
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]interface{}{
				"nextSyncToken": "sync-token-next-456",
				"items": []map[string]interface{}{
					{
						"id":          "evt-1",
						"summary":     "Sprint Planning Meet",
						"description": "Weekly sprint planning https://meet.google.com/abc-defg-hij",
						"start": map[string]interface{}{
							"dateTime": "2026-11-23T10:00:00Z",
						},
						"end": map[string]interface{}{
							"dateTime": "2026-11-23T11:00:00Z",
						},
						"organizer": map[string]interface{}{
							"email": "pm@youten.ai",
						},
						"attendees": []map[string]interface{}{
							{
								"email":          "me@youten.ai",
								"self":           true,
								"responseStatus": "accepted",
							},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// Custom transport redirecting googleapis to mock server
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			newURL := server.URL + req.URL.Path + "?" + req.URL.RawQuery
			newReq, _ := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
			newReq.Header = req.Header
			return http.DefaultTransport.RoundTrip(newReq)
		}),
	}

	adapter := googlecalendar.NewAdapter(googlecalendar.Config{
		HTTPClient: client,
	})

	res, err := adapter.FetchEvents(context.Background(), "mock-access-token", "", time.Now())
	if err != nil {
		t.Fatalf("FetchEvents returned unexpected error: %v", err)
	}

	if res.NextSyncToken != "sync-token-next-456" {
		t.Errorf("expected NextSyncToken sync-token-next-456, got %s", res.NextSyncToken)
	}
	if len(res.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(res.Events))
	}

	evt := res.Events[0]
	if evt.ID != "evt-1" || evt.Title != "Sprint Planning Meet" {
		t.Errorf("unexpected event details: %+v", evt)
	}
	if evt.MeetingPlatform != constants.MeetingPlatformGoogleMeet || evt.MeetingURL != "https://meet.google.com/abc-defg-hij" {
		t.Errorf("unexpected meeting extraction: url=%s, platform=%s", evt.MeetingURL, evt.MeetingPlatform)
	}
	if evt.UserResponseStatus != "accepted" {
		t.Errorf("expected accepted status, got %s", evt.UserResponseStatus)
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

