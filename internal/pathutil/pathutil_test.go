package pathutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// symlinkedRoot mimics macOS, where /var is a symlink to /private/var: the same
// folder has two spellings, so containment cannot be a string comparison.
func symlinkedRoot(t *testing.T) (link, real string) {
	t.Helper()
	base := t.TempDir()
	real = filepath.Join(base, "private", "project")
	if err := os.MkdirAll(filepath.Join(real, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	link = filepath.Join(base, "project")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	os.WriteFile(filepath.Join(real, "src", "main.go"), []byte("package main\n"), 0o644)
	return link, real
}

func TestContainAcceptsBothSpellingsOfTheSameFolder(t *testing.T) {
	link, real := symlinkedRoot(t)

	for _, root := range []string{link, real} {
		for _, rel := range []string{
			"src/main.go",                         // relative
			filepath.Join(link, "src", "main.go"), // absolute, symlinked spelling
			filepath.Join(real, "src", "main.go"), // absolute, resolved spelling
			"src/../src/main.go",                  // takes a detour, still inside
		} {
			abs, err := Contain(root, rel)
			if err != nil {
				t.Errorf("Contain(%q, %q) = %v, want allowed", root, rel, err)
				continue
			}
			// Whichever spelling came in, the result is under the root we were given.
			if !strings.HasPrefix(abs, filepath.Clean(root)+string(filepath.Separator)) {
				t.Errorf("Contain(%q, %q) = %q, want a path under the root", root, rel, abs)
			}
			if _, err := os.Stat(abs); err != nil {
				t.Errorf("Contain(%q, %q) = %q, which does not exist: %v", root, rel, abs, err)
			}
		}
	}
}

func TestContainRefusesEscapes(t *testing.T) {
	link, real := symlinkedRoot(t)
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s"), 0o600)
	// A symlink inside the project that points out of it.
	os.Symlink(outside, filepath.Join(real, "escape"))

	for _, root := range []string{link, real} {
		for _, rel := range []string{
			"..",
			"../secret.txt",
			outside,
			filepath.Join(outside, "secret.txt"),
			"escape/secret.txt",
			"src/../../escape",
			"~/secret.txt",
		} {
			if abs, err := Contain(root, rel); err == nil {
				t.Errorf("Contain(%q, %q) = %q, want refused", root, rel, abs)
			}
		}
	}
}

func TestRelUsesTheProjectSpelling(t *testing.T) {
	link, real := symlinkedRoot(t)
	// A path recorded in either spelling reads back as a project-relative path.
	if got := Rel(link, filepath.Join(real, "src", "main.go")); got != "src/main.go" {
		t.Errorf("Rel(link, real path) = %q", got)
	}
	if got := Rel(real, filepath.Join(link, "src", "main.go")); got != "src/main.go" {
		t.Errorf("Rel(real, link path) = %q", got)
	}
	// Something genuinely outside keeps its absolute path rather than ../..
	outside := t.TempDir()
	if got := Rel(link, filepath.Join(outside, "x")); !filepath.IsAbs(got) {
		t.Errorf("Rel(root, outside) = %q, want an absolute path", got)
	}
}

func TestWithinAndSameFolder(t *testing.T) {
	link, real := symlinkedRoot(t)
	if !SameFolder(link, real) {
		t.Error("the symlink and its target are the same folder")
	}
	if !Within(link, filepath.Join(real, "src")) || !Within(real, filepath.Join(link, "src")) {
		t.Error("Within should see through the symlink in both directions")
	}
	if Within(link, t.TempDir()) {
		t.Error("an unrelated folder is not inside the project")
	}
	// Sibling folders that merely share a prefix are not inside.
	if Within(filepath.Join(real, "src"), filepath.Join(real, "src-other")) {
		t.Error("src-other is not inside src")
	}
}

func TestResolvedHandlesMissingPaths(t *testing.T) {
	link, real := symlinkedRoot(t)
	missing := filepath.Join(link, "does", "not", "exist.txt")
	want := filepath.Join(real, "does", "not", "exist.txt")
	if got := Resolved(missing); got != want {
		t.Errorf("Resolved(%q) = %q, want %q", missing, got, want)
	}
}
