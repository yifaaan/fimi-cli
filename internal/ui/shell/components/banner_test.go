package components

import (
	"strings"
	"testing"
)

func TestRenderBannerIncludesModelAndWorkDir(t *testing.T) {
	got := RenderBanner(BannerInfo{
		SessionID:     "session-12345678",
		SessionReused: true,
		ModelName:     "glm-5",
		AppVersion:    "1.2.3",
		WorkDir:       "/tmp/fimi-project",
		LastRole:      "assistant",
		LastSummary:   "picked up from the latest checkpoint",
	})

	for _, want := range []string{
		"fimi-cli v1.2.3",
		"glm-5 · continue session",
		"/tmp/fimi-project",
		"resume session-1234",
		"assistant: picked up from the latest checkpoint",
		"Enter send | /help /clear /exit | Ctrl+C quit",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderBanner() missing %q in %q", want, got)
		}
	}
}

func TestRenderBannerOmitsUserSummaryLine(t *testing.T) {
	got := RenderBanner(BannerInfo{
		LastRole:    "user",
		LastSummary: "please continue from here",
	})

	if strings.Contains(got, "user: please continue from here") {
		t.Fatalf("RenderBanner() = %q, want no user summary line", got)
	}
}

func TestRenderBannerFallsBackWithoutSession(t *testing.T) {
	got := RenderBanner(BannerInfo{})

	if !strings.Contains(got, "interactive coding shell") {
		t.Fatalf("RenderBanner() = %q, want interactive coding shell subtitle", got)
	}
	if strings.Contains(got, "resume ") || strings.Contains(got, "new ") {
		t.Fatalf("RenderBanner() = %q, want no session line", got)
	}
}

func TestRenderBannerMarksDevBuild(t *testing.T) {
	got := RenderBanner(BannerInfo{
		AppVersion: "dev",
		ModelName:  "glm-5",
	})

	if !strings.Contains(got, "glm-5 · dev build") {
		t.Fatalf("RenderBanner() = %q, want dev build marker", got)
	}
}
