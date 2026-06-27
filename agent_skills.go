package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	agentSkillsRoot          = ".agents"
	agentSkillsMaxDepth      = 4
	agentSkillsMaxFiles      = 40
	agentSkillsMaxFileBytes  = 64 * 1024
	agentSkillsMaxTotalBytes = 256 * 1024
)

type agentSkillsEnvelope struct {
	Skills         []agentSkill `json:"skills"`
	Truncated      bool         `json:"truncated"`
	MaxFiles       int          `json:"max_files"`
	MaxFileBytes   int          `json:"max_file_bytes"`
	MaxTotalBytes  int          `json:"max_total_bytes"`
	TotalBytesRead int          `json:"total_bytes_read"`
}

type agentSkill struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	Size      int    `json:"size"`
	Truncated bool   `json:"truncated"`
}

func listAgentSkills(workspace string) (string, error) {
	root, err := resolveWorkspacePath(workspace, agentSkillsRoot)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		return marshalAgentSkillsEnvelope(agentSkillsEnvelope{})
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New(".agents is not a directory")
	}

	paths := []string{}
	truncated := false
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." {
			return nil
		}
		depth := strings.Count(filepath.ToSlash(rel), "/") + 1
		if entry.IsDir() {
			if depth > agentSkillsMaxDepth || entry.Type()&os.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !isAgentSkillMarkdown(entry.Name()) {
			return nil
		}
		if depth > agentSkillsMaxDepth {
			return nil
		}
		if len(paths) >= agentSkillsMaxFiles {
			truncated = true
			return filepath.SkipAll
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(paths, func(i, j int) bool {
		return strings.ToLower(filepath.ToSlash(paths[i])) < strings.ToLower(filepath.ToSlash(paths[j]))
	})

	envelope := agentSkillsEnvelope{
		Skills:        []agentSkill{},
		Truncated:     truncated,
		MaxFiles:      agentSkillsMaxFiles,
		MaxFileBytes:  agentSkillsMaxFileBytes,
		MaxTotalBytes: agentSkillsMaxTotalBytes,
	}
	for _, path := range paths {
		if envelope.TotalBytesRead >= agentSkillsMaxTotalBytes {
			envelope.Truncated = true
			break
		}
		remaining := agentSkillsMaxTotalBytes - envelope.TotalBytesRead
		limit := agentSkillsMaxFileBytes
		if remaining < limit {
			limit = remaining
		}
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(file, int64(limit)+1))
		_ = file.Close()
		if readErr != nil {
			continue
		}
		fileTruncated := len(data) > limit
		if fileTruncated {
			data = data[:limit]
			envelope.Truncated = true
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		envelope.TotalBytesRead += len(data)
		envelope.Skills = append(envelope.Skills, agentSkill{
			ID:        skillIDFromPath(rel),
			Name:      skillNameFromPath(rel),
			Path:      rel,
			Content:   content,
			Size:      len(data),
			Truncated: fileTruncated,
		})
	}
	return marshalAgentSkillsEnvelope(envelope)
}

func isAgentSkillMarkdown(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".md")
}

func skillIDFromPath(path string) string {
	id := strings.Trim(strings.ToLower(path), "/")
	id = strings.TrimPrefix(id, ".agents/")
	id = strings.TrimSuffix(id, ".md")
	id = strings.ReplaceAll(id, "/", "__")
	id = strings.ReplaceAll(id, "\\", "__")
	return id
}

func skillNameFromPath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) >= 3 && strings.EqualFold(parts[len(parts)-1], "skill.md") {
		return parts[len(parts)-2]
	}
	name := strings.TrimSuffix(parts[len(parts)-1], filepath.Ext(parts[len(parts)-1]))
	if strings.TrimSpace(name) == "" {
		return path
	}
	return name
}

func marshalAgentSkillsEnvelope(envelope agentSkillsEnvelope) (string, error) {
	if envelope.Skills == nil {
		envelope.Skills = []agentSkill{}
	}
	if envelope.MaxFiles == 0 {
		envelope.MaxFiles = agentSkillsMaxFiles
	}
	if envelope.MaxFileBytes == 0 {
		envelope.MaxFileBytes = agentSkillsMaxFileBytes
	}
	if envelope.MaxTotalBytes == 0 {
		envelope.MaxTotalBytes = agentSkillsMaxTotalBytes
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
