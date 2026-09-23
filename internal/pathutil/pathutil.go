// Package pathutil holds the one rule every tool depends on: a path is only
// usable if it stays inside the project folder. It lives on its own so the
// projects service, the built-in tools and anything added later share exactly
// the same comparison.
package pathutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolved returns path with symlinks resolved as far as the file system can:
// the deepest part that exists is resolved, the rest is appended unchanged.
// This matters on macOS, where /var, /tmp and /etc are symlinks into /private,
// so the same folder has two spellings and a plain string comparison rejects
// paths that are in fact inside the project.
func Resolved(path string) string {
	path = filepath.Clean(path)
	probe, rest := path, ""
	for {
		if resolved, err := filepath.EvalSymlinks(probe); err == nil {
			return filepath.Join(resolved, rest)
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return path
		}
		rest = filepath.Join(filepath.Base(probe), rest)
		probe = parent
	}
}

// Within reports whether target is root or inside it, comparing both in their
// resolved form.
func Within(root, target string) bool {
	root, target = Resolved(root), Resolved(target)
	if target == root {
		return true
	}
	return strings.HasPrefix(target, root+string(filepath.Separator))
}

// Contain joins rel to root and refuses anything that leaves it, including a
// path that reaches out through a symlink. rel may be relative to root or an
// absolute path in either spelling; the result is absolute and cleaned, in the
// same spelling as root so messages and recorded paths stay consistent.
func Contain(root, rel string) (string, error) {
	root = filepath.Clean(root)
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return root, nil
	}
	if strings.HasPrefix(rel, "~") {
		return "", fmt.Errorf("%s is outside the project folder %s", rel, root)
	}
	target := rel
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	target = filepath.Clean(target)

	if !Within(root, target) {
		return "", fmt.Errorf("%s is outside the project folder %s; the agent can only read and change files inside the open project", rel, root)
	}
	// Re-express an absolute argument in the project's own spelling, so
	// /private/var/… and /var/… both come back as paths under root.
	if filepath.IsAbs(rel) {
		if inside, err := filepath.Rel(Resolved(root), Resolved(target)); err == nil && inside != "." {
			target = filepath.Join(root, inside)
		} else if err == nil {
			target = root
		}
	}
	// A symlink inside the project may still point out of it.
	if resolved := Resolved(target); !Within(root, resolved) {
		return "", fmt.Errorf("%s points to %s, outside the project folder", rel, resolved)
	}
	return target, nil
}

// Rel is the project-relative, slash-separated form of abs, for display and for
// the paths recorded with file changes.
func Rel(root, abs string) string {
	if rel, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	if rel, err := filepath.Rel(Resolved(root), Resolved(abs)); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return abs
}

// SameFolder reports whether two paths name the same folder, in any spelling.
func SameFolder(a, b string) bool { return Resolved(a) == Resolved(b) }

// Exists is a small helper used when validating a project root.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
