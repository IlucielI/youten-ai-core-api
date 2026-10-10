package constants_test

import (
	"testing"

	"code-base-golang/internal/constants"
)

func TestIsValidTemplate(t *testing.T) {
	expectedTemplates := []string{
		constants.TemplateKeyMOM,
		constants.TemplateKeyOneOnOne,
		constants.TemplateKeyInterview,
		constants.TemplateKeyTechReview,
		constants.TemplateKeySalesDiscovery,
		constants.TemplateKeyDailyStandup,
		constants.TemplateKeyGeneral,
		constants.TemplateKeyPodcast,
		constants.TemplateKeyLecture,
		constants.TemplateKeyMusicLyrics,
		constants.TemplateKeyResearchDeepdive,
	}

	for _, tmpl := range expectedTemplates {
		if !constants.IsValidTemplate(tmpl) {
			t.Errorf("expected IsValidTemplate(%q) to be true", tmpl)
		}
	}

	invalidTemplates := []string{
		"INVALID",
		"",
		"random_key",
		"podcast", // case sensitive check
	}

	for _, inv := range invalidTemplates {
		if constants.IsValidTemplate(inv) {
			t.Errorf("expected IsValidTemplate(%q) to be false", inv)
		}
	}
}
