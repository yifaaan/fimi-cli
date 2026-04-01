package mcp

import "testing"

func TestMergeCommandEnvPreservesBaseAndAppliesOverrides(t *testing.T) {
	got := mergeCommandEnv(
		[]string{"PATH=/bin", "HOME=/tmp/home", "LANG=en_US.UTF-8"},
		map[string]string{
			"PATH":       "/custom/bin",
			"OPENAI_KEY": "secret",
		},
	)

	env := make(map[string]string, len(got))
	for _, item := range got {
		for i := 0; i < len(item); i++ {
			if item[i] != '=' {
				continue
			}
			env[item[:i]] = item[i+1:]
			break
		}
	}

	if env["PATH"] != "/custom/bin" {
		t.Fatalf("PATH = %q, want %q", env["PATH"], "/custom/bin")
	}
	if env["HOME"] != "/tmp/home" {
		t.Fatalf("HOME = %q, want %q", env["HOME"], "/tmp/home")
	}
	if env["OPENAI_KEY"] != "secret" {
		t.Fatalf("OPENAI_KEY = %q, want %q", env["OPENAI_KEY"], "secret")
	}
}
