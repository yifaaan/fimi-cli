package changelog

import (
	"testing"
)

func TestParseReleasesExtractsVersionAndDate(t *testing.T) {
	content := `## [0.35] - 2025-10-22

### Changed

- Minor UI improvements
- Auto download ripgrep if not found in the system

## [0.34] - 2025-10-21

### Added

- Add /update meta command
`

	entries := ParseReleases(content, 10)
	if len(entries) != 2 {
		t.Fatalf("ParseReleases() returned %d entries, want 2", len(entries))
	}

	if entries[0].Version != "0.35" {
		t.Fatalf("entries[0].Version = %q, want %q", entries[0].Version, "0.35")
	}
	if entries[0].Date != "2025-10-22" {
		t.Fatalf("entries[0].Date = %q, want %q", entries[0].Date, "2025-10-22")
	}
	if entries[1].Version != "0.34" {
		t.Fatalf("entries[1].Version = %q, want %q", entries[1].Version, "0.34")
	}
}

func TestParseReleasesCollectsBulletsAcrossSections(t *testing.T) {
	content := `## [0.35] - 2025-10-22

### Changed

- Minor UI improvements
- Auto download ripgrep

### Fixed

- Fix logging redirection

## [0.34] - 2025-10-21

### Added

- Add /update meta command
`

	entries := ParseReleases(content, 10)
	if len(entries) != 2 {
		t.Fatalf("ParseReleases() returned %d entries, want 2", len(entries))
	}

	// First entry should have 3 bullets (2 from Changed, 1 from Fixed)
	if len(entries[0].Bullets) != 3 {
		t.Fatalf("entries[0].Bullets len = %d, want 3", len(entries[0].Bullets))
	}
	if entries[0].Bullets[0] != "Minor UI improvements" {
		t.Fatalf("entries[0].Bullets[0] = %q, want %q", entries[0].Bullets[0], "Minor UI improvements")
	}
	if entries[0].Bullets[2] != "Fix logging redirection" {
		t.Fatalf("entries[0].Bullets[2] = %q, want %q", entries[0].Bullets[2], "Fix logging redirection")
	}
}

func TestParseReleasesRespectsLimit(t *testing.T) {
	content := `## [0.35] - 2025-10-22

### Changed

- Minor UI improvements

## [0.34] - 2025-10-21

### Added

- Add /update meta command

## [0.33] - 2025-10-18

### Fixed

- Fix logging
`

	entries := ParseReleases(content, 2)
	if len(entries) != 2 {
		t.Fatalf("ParseReleases() returned %d entries, want 2", len(entries))
	}
	if entries[0].Version != "0.35" {
		t.Fatalf("entries[0].Version = %q, want 0.35", entries[0].Version)
	}
	if entries[1].Version != "0.34" {
		t.Fatalf("entries[1].Version = %q, want 0.34", entries[1].Version)
	}
}

func TestParseReleasesReturnsEmptyForNoContent(t *testing.T) {
	entries := ParseReleases("", 10)
	if len(entries) != 0 {
		t.Fatalf("ParseReleases(\"\") returned %d entries, want 0", len(entries))
	}
}
