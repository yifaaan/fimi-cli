package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const MetadataFileName = "metadata.json"

// Metadata 表示 session 包当前维护的最小元数据视图。
// 这里只保存 continue 语义真正需要的索引信息，不把运行时状态塞进来。
type Metadata struct {
	WorkDirs []WorkDirMetadata `json:"work_dirs"`
}

// WorkDirMetadata 表示某个工作目录的 session 索引。
type WorkDirMetadata struct {
	Path          string `json:"path"`
	LastSessionID string `json:"last_session_id,omitempty"`
}

func metadataFile() (string, error) {
	stateDir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(stateDir, MetadataFileName), nil
}

func loadMetadata() (Metadata, error) {
	metadataPath, err := metadataFile()
	if err != nil {
		return Metadata{}, err
	}

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Metadata{}, nil
		}
		return Metadata{}, fmt.Errorf("read metadata file %q: %w", metadataPath, err)
	}

	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return Metadata{}, fmt.Errorf("decode metadata file %q: %w", metadataPath, err)
	}

	return meta, nil
}

func saveMetadata(meta Metadata) error {
	metadataPath, err := metadataFile()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(metadataPath), 0o755); err != nil {
		return fmt.Errorf("create metadata dir for %q: %w", metadataPath, err)
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata file %q: %w", metadataPath, err)
	}

	if err := os.WriteFile(metadataPath, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write metadata file %q: %w", metadataPath, err)
	}

	return nil
}

func setLastSessionID(workDir, sessionID string) error {
	meta, err := loadMetadata()
	if err != nil {
		return err
	}

	entry := meta.workDir(workDir)
	entry.LastSessionID = sessionID

	return saveMetadata(meta)
}

func lastSessionIDForWorkDir(workDir string) (string, error) {
	meta, err := loadMetadata()
	if err != nil {
		return "", err
	}

	for _, entry := range meta.WorkDirs {
		if entry.Path == workDir {
			return entry.LastSessionID, nil
		}
	}

	return "", nil
}

func (m *Metadata) workDir(path string) *WorkDirMetadata {
	for i := range m.WorkDirs {
		if m.WorkDirs[i].Path == path {
			return &m.WorkDirs[i]
		}
	}

	m.WorkDirs = append(m.WorkDirs, WorkDirMetadata{Path: path})
	return &m.WorkDirs[len(m.WorkDirs)-1]
}
