package dtos_test

import (
	"testing"

	"code-base-golang/internal/dtos"
)

func TestSpeakerDirectoryResponse_Structure(t *testing.T) {
	resp := dtos.SpeakerDirectoryResponse{
		Count: 2,
		Speakers: []dtos.SpeakerSummary{
			{
				Name:          "Alice",
				TotalMeetings: 3,
				TotalTalkTime: 450.5,
			},
			{
				Name:          "Bob",
				TotalMeetings: 1,
				TotalTalkTime: 120.0,
			},
		},
	}

	if resp.Count != 2 {
		t.Errorf("expected count 2, got %d", resp.Count)
	}
	if len(resp.Speakers) != 2 {
		t.Fatalf("expected 2 speakers, got %d", len(resp.Speakers))
	}
	if resp.Speakers[0].Name != "Alice" || resp.Speakers[0].TotalMeetings != 3 || resp.Speakers[0].TotalTalkTime != 450.5 {
		t.Errorf("unexpected speaker 0 data: %+v", resp.Speakers[0])
	}
	if resp.Speakers[1].Name != "Bob" || resp.Speakers[1].TotalMeetings != 1 || resp.Speakers[1].TotalTalkTime != 120.0 {
		t.Errorf("unexpected speaker 1 data: %+v", resp.Speakers[1])
	}
}
