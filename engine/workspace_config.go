package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DefaultConfigFileName is the optional configuration file checked inside the workspace root.
	DefaultConfigFileName = "engine.json"
	// DefaultTutorialStateFileName stores persisted tutorial dismissal state inside ConfigDir.
	DefaultTutorialStateFileName = "tutorial_state.json"
)

// WorkspaceConfig defines configurable directory roots and startup metadata loaded from engine.json.
type WorkspaceConfig struct {
	RootDir     string `json:"root_dir,omitempty"`
	AssetsDir   string `json:"assets_dir,omitempty"`
	ScriptsDir  string `json:"scripts_dir,omitempty"`
	PrefabsDir  string `json:"prefabs_dir,omitempty"`
	ConfigDir   string `json:"config_dir,omitempty"`
	WindowTitle string `json:"window_title,omitempty"`
}

// DefaultWorkspaceConfig returns default relative subdirectory names for a Project Engine workspace.
func DefaultWorkspaceConfig() WorkspaceConfig {
	return WorkspaceConfig{
		RootDir:     ".",
		AssetsDir:   "assets",
		ScriptsDir:  "scripts",
		PrefabsDir:  "prefabs",
		ConfigDir:   "config",
		WindowTitle: "Project Engine - Kitchen Sink Demo",
	}
}

// ResolvedWorkspacePaths holds resolved filesystem paths for a Project Engine game/script/config/data root.
type ResolvedWorkspacePaths struct {
	RootDir           string
	ConfigFile        string
	ConfigLoaded      bool
	AssetsDir         string
	ScriptsDir        string
	PrefabsDir        string
	ConfigDir         string
	TutorialStateFile string
	WindowTitle       string
}

// WorkspaceVerificationError reports missing or invalid required directories during startup verification.
type WorkspaceVerificationError struct {
	RootDir      string
	MissingDirs  []string
	InvalidPaths []string
}

func (e *WorkspaceVerificationError) Error() string {
	var parts []string
	if len(e.MissingDirs) > 0 {
		parts = append(parts, "missing directories: "+strings.Join(e.MissingDirs, ", "))
	}
	if len(e.InvalidPaths) > 0 {
		parts = append(parts, "not directories: "+strings.Join(e.InvalidPaths, ", "))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("workspace verification failed for root %q", e.RootDir)
	}
	return fmt.Sprintf(
		"workspace verification failed for root %q (%s). Pass --root <dir> to select an existing project root, or --init-dirs to create the standard directory structure",
		e.RootDir,
		strings.Join(parts, "; "),
	)
}

// LoadWorkspaceConfig resolves workspace directories from defaults, optional/explicit config JSON,
// and CLI root overrides. When configPath is non-empty, it must exist and be valid JSON.
func LoadWorkspaceConfig(rootFlag string, rootExplicit bool, configPath string) (WorkspaceConfig, ResolvedWorkspacePaths, error) {
	cfg := DefaultWorkspaceConfig()
	trimmedConfig := strings.TrimSpace(configPath)
	trimmedRoot := strings.TrimSpace(rootFlag)
	if trimmedRoot == "" {
		trimmedRoot = "."
	}

	effectiveConfigFile := ""
	configLoaded := false

	if trimmedConfig != "" {
		effectiveConfigFile = filepath.Clean(trimmedConfig)
		loadedCfg, err := readWorkspaceConfigFile(effectiveConfigFile)
		if err != nil {
			return cfg, ResolvedWorkspacePaths{}, err
		}
		mergeWorkspaceConfig(&cfg, loadedCfg)
		configLoaded = true
		if !rootExplicit {
			if strings.TrimSpace(loadedCfg.RootDir) != "" {
				if filepath.IsAbs(loadedCfg.RootDir) {
					trimmedRoot = filepath.Clean(loadedCfg.RootDir)
				} else {
					trimmedRoot = filepath.Clean(filepath.Join(filepath.Dir(effectiveConfigFile), loadedCfg.RootDir))
				}
			} else {
				trimmedRoot = filepath.Clean(filepath.Dir(effectiveConfigFile))
			}
		}
	} else {
		candidate := filepath.Join(trimmedRoot, DefaultConfigFileName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			effectiveConfigFile = filepath.Clean(candidate)
			loadedCfg, err := readWorkspaceConfigFile(effectiveConfigFile)
			if err != nil {
				return cfg, ResolvedWorkspacePaths{}, err
			}
			mergeWorkspaceConfig(&cfg, loadedCfg)
			configLoaded = true
			if !rootExplicit && strings.TrimSpace(loadedCfg.RootDir) != "" {
				if filepath.IsAbs(loadedCfg.RootDir) {
					trimmedRoot = filepath.Clean(loadedCfg.RootDir)
				} else {
					trimmedRoot = filepath.Clean(filepath.Join(trimmedRoot, loadedCfg.RootDir))
				}
			}
		} else {
			effectiveConfigFile = filepath.Clean(candidate)
		}
	}

	cfg.RootDir = filepath.Clean(trimmedRoot)

	paths := ResolvedWorkspacePaths{
		RootDir:      cfg.RootDir,
		ConfigFile:   effectiveConfigFile,
		ConfigLoaded: configLoaded,
		AssetsDir:    resolveSubdir(cfg.RootDir, cfg.AssetsDir, "assets"),
		ScriptsDir:   resolveSubdir(cfg.RootDir, cfg.ScriptsDir, "scripts"),
		PrefabsDir:   resolveSubdir(cfg.RootDir, cfg.PrefabsDir, "prefabs"),
		ConfigDir:    resolveSubdir(cfg.RootDir, cfg.ConfigDir, "config"),
		WindowTitle:  cfg.WindowTitle,
	}
	paths.TutorialStateFile = filepath.Join(paths.ConfigDir, DefaultTutorialStateFileName)
	return cfg, paths, nil
}

// VerifyWorkspaceStructure checks that RootDir and all required project subdirectories exist on disk.
func VerifyWorkspaceStructure(paths ResolvedWorkspacePaths) error {
	var missing []string
	var invalid []string

	checkDir := func(label, p string) {
		info, err := os.Stat(p)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				missing = append(missing, fmt.Sprintf("%s (%s)", label, filepath.ToSlash(p)))
			} else {
				invalid = append(invalid, fmt.Sprintf("%s (%s: %v)", label, filepath.ToSlash(p), err))
			}
			return
		}
		if !info.IsDir() {
			invalid = append(invalid, fmt.Sprintf("%s (%s)", label, filepath.ToSlash(p)))
		}
	}

	checkDir("root", paths.RootDir)
	checkDir("assets", paths.AssetsDir)
	checkDir("scripts", paths.ScriptsDir)
	checkDir("prefabs", paths.PrefabsDir)
	checkDir("config", paths.ConfigDir)

	if len(missing) > 0 || len(invalid) > 0 {
		return &WorkspaceVerificationError{
			RootDir:      paths.RootDir,
			MissingDirs:  missing,
			InvalidPaths: invalid,
		}
	}
	return nil
}

// InitWorkspaceDirs creates the standard Project Engine directory tree under paths.
func InitWorkspaceDirs(paths ResolvedWorkspacePaths) ([]string, error) {
	targets := []string{
		paths.RootDir,
		paths.AssetsDir,
		filepath.Join(paths.AssetsDir, "models", "props"),
		filepath.Join(paths.AssetsDir, "maps"),
		paths.ScriptsDir,
		filepath.Join(paths.ScriptsDir, "components"),
		filepath.Join(paths.ScriptsDir, "props"),
		filepath.Join(paths.ScriptsDir, "ui"),
		paths.PrefabsDir,
		paths.ConfigDir,
	}
	created := make([]string, 0, len(targets))
	for _, dir := range targets {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			created = append(created, filepath.Clean(dir))
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return created, fmt.Errorf("init workspace dir %q: %w", dir, err)
		}
	}
	return created, nil
}

// ResolvePath resolves relOrAbs relative to RootDir unless already absolute.
// Paths beginning with "assets/" map into AssetsDir so custom assets_dir overrides work cleanly.
func (p ResolvedWorkspacePaths) ResolvePath(relOrAbs string) string {
	trimmed := strings.TrimSpace(relOrAbs)
	if trimmed == "" {
		return ""
	}
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed)
	}
	slash := filepath.ToSlash(filepath.Clean(trimmed))
	if slash == "assets" {
		return filepath.Clean(p.AssetsDir)
	}
	if strings.HasPrefix(slash, "assets/") {
		rel := strings.TrimPrefix(slash, "assets/")
		return filepath.Clean(filepath.Join(p.AssetsDir, filepath.FromSlash(rel)))
	}
	if slash == "scripts" {
		return filepath.Clean(p.ScriptsDir)
	}
	if strings.HasPrefix(slash, "scripts/") {
		rel := strings.TrimPrefix(slash, "scripts/")
		return filepath.Clean(filepath.Join(p.ScriptsDir, filepath.FromSlash(rel)))
	}
	if slash == "prefabs" {
		return filepath.Clean(p.PrefabsDir)
	}
	if strings.HasPrefix(slash, "prefabs/") {
		rel := strings.TrimPrefix(slash, "prefabs/")
		return filepath.Clean(filepath.Join(p.PrefabsDir, filepath.FromSlash(rel)))
	}
	if slash == "config" {
		return filepath.Clean(p.ConfigDir)
	}
	if strings.HasPrefix(slash, "config/") {
		rel := strings.TrimPrefix(slash, "config/")
		return filepath.Clean(filepath.Join(p.ConfigDir, filepath.FromSlash(rel)))
	}
	return filepath.Clean(filepath.Join(p.RootDir, filepath.FromSlash(slash)))
}

func readWorkspaceConfigFile(path string) (WorkspaceConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return WorkspaceConfig{}, fmt.Errorf("workspace config %q: %w", path, err)
	}
	var cfg WorkspaceConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return WorkspaceConfig{}, fmt.Errorf("parse workspace config %q: %w", path, err)
	}
	return cfg, nil
}

func mergeWorkspaceConfig(dst *WorkspaceConfig, src WorkspaceConfig) {
	if s := strings.TrimSpace(src.RootDir); s != "" {
		dst.RootDir = s
	}
	if s := strings.TrimSpace(src.AssetsDir); s != "" {
		dst.AssetsDir = s
	}
	if s := strings.TrimSpace(src.ScriptsDir); s != "" {
		dst.ScriptsDir = s
	}
	if s := strings.TrimSpace(src.PrefabsDir); s != "" {
		dst.PrefabsDir = s
	}
	if s := strings.TrimSpace(src.ConfigDir); s != "" {
		dst.ConfigDir = s
	}
	if s := strings.TrimSpace(src.WindowTitle); s != "" {
		dst.WindowTitle = s
	}
}

func resolveSubdir(root, candidate, fallback string) string {
	dir := strings.TrimSpace(candidate)
	if dir == "" {
		dir = fallback
	}
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}
	return filepath.Clean(filepath.Join(root, dir))
}
