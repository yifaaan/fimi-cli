package changelog

import (
	"regexp"
	"strings"
)

// ReleaseEntry represents a parsed changelog version entry.
type ReleaseEntry struct {
	Version string
	Date    string
	Bullets []string
}

// ParseReleases parses changelog markdown and returns release entries.
// Returns entries in reverse chronological order (newest first), capped at limit.
func ParseReleases(content string, limit int) []ReleaseEntry {
	if content == "" || limit <= 0 {
		return nil
	}

	// Find all version headers: ## [version] - date
	headerPattern := regexp.MustCompile(`(?m)^## \[([^\]]+)\] - ([^\n]+)`)
	matches := headerPattern.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return nil
	}

	var entries []ReleaseEntry
	for i, match := range matches {
		version := content[match[2]:match[3]]
		date := strings.TrimSpace(content[match[4]:match[5]])

		// Determine the end of this section (start of next header or end of content)
		var sectionEnd int
		if i < len(matches)-1 {
			sectionEnd = matches[i+1][0]
		} else {
			sectionEnd = len(content)
		}

		section := content[match[0]:sectionEnd]
		lines := strings.Split(section, "\n")

		entry := ReleaseEntry{
			Version: version,
			Date:    date,
			Bullets: nil,
		}

		// Collect bullet lines across all subsections
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- ") {
				bullet := strings.TrimPrefix(line, "- ")
				entry.Bullets = append(entry.Bullets, bullet)
			}
		}

		if len(entry.Bullets) > 0 {
			entries = append(entries, entry)
		}
	}

	// Entries are already in reverse chronological order from file (newest first)
	// Cap at limit
	if len(entries) > limit {
		entries = entries[:limit]
	}

	return entries
}
