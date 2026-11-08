package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ArchiveDir keeps what "Archive finished" took out of a project's backlog (WFL-004,
// OQ-041), as archive/<project>/<when>/<the file's path in the project>.
const ArchiveDir = "archive"

// ArchiveFile keeps one file of a project's backlog in the data folder.
func ArchiveFile(dir, projectID, when, rel string, data []byte) error {
	if !idPattern.MatchString(projectID) && !filepath.IsLocal(projectID) {
		return fmt.Errorf("bad project %q", projectID)
	}
	if !filepath.IsLocal(when) || !filepath.IsLocal(filepath.FromSlash(rel)) {
		return fmt.Errorf("bad archive path %q", rel)
	}
	path := filepath.Join(dir, ArchiveDir, projectID, when, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return writeAtomic(path, data)
}
