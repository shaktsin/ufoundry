package identity

import "testing"

func TestCompactStatusSkipsWarnings(t *testing.T) {
	got := compactStatus("WARNING: no PATH alias\nLogged in using ChatGPT\n", "fallback")
	if got != "Logged in using ChatGPT" {
		t.Fatalf("compactStatus = %q", got)
	}
}

func TestCompactStatusFallback(t *testing.T) {
	if got := compactStatus("\n WARNING: unavailable \n", "Not signed in"); got != "Not signed in" {
		t.Fatalf("compactStatus = %q", got)
	}
}
