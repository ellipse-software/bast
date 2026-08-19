package platform

import "path/filepath"

func HomeRelative(path, home string) string {
	if path == "" || home == "" || !PathContained(home, path) {
		return path
	}
	rel, err := filepath.Rel(home, path)
	if err != nil || rel == "." {
		return path
	}
	return "~/" + filepath.ToSlash(rel)
}
