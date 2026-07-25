package depolicy

import (
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/mod/modfile"
)

func LoadProjectConfig(configPath string) (*CompiledConfig, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	module, err := ReadModuleInfo(cfg.Path)
	if err != nil {
		return nil, err
	}
	compiled, err := CompileConfig(cfg, module)
	if err != nil {
		return nil, &ConfigError{Path: cfg.Path, Message: err.Error()}
	}
	return compiled, nil
}

func ReadModuleInfo(configPath string) (ModuleInfo, error) {
	start := filepath.Dir(configPath)
	for {
		goMod := filepath.Join(start, "go.mod")
		if _, err := os.Stat(goMod); err == nil {
			data, err := os.ReadFile(goMod)
			if err != nil {
				return ModuleInfo{}, err
			}
			parsed, err := modfile.Parse(goMod, data, nil)
			if err != nil {
				return ModuleInfo{}, &ConfigError{Path: goMod, Message: err.Error()}
			}
			if parsed.Module == nil || parsed.Module.Mod.Path == "" {
				return ModuleInfo{}, &ConfigError{Path: goMod, Message: "go.mod is missing module directive"}
			}
			return ModuleInfo{
				Path:    parsed.Module.Mod.Path,
				RootDir: start,
				GoMod:   goMod,
			}, nil
		} else if !os.IsNotExist(err) {
			return ModuleInfo{}, err
		}
		parent := filepath.Dir(start)
		if parent == start {
			return ModuleInfo{}, &ConfigError{Path: configPath, Message: "corresponding go.mod was not found"}
		}
		start = parent
	}
}

func ClassifyImportPath(module ModuleInfo, importPath string) PackageRef {
	switch {
	case importPath == module.Path:
		return PackageRef{Kind: PackageKindLocal, Path: "", ImportPath: importPath}
	case strings.HasPrefix(importPath, module.Path+"/"):
		return PackageRef{
			Kind:       PackageKindLocal,
			Path:       strings.TrimPrefix(importPath, module.Path+"/"),
			ImportPath: importPath,
		}
	case IsStandardPackage(importPath):
		return PackageRef{Kind: PackageKindStd, Path: importPath, ImportPath: importPath}
	default:
		return PackageRef{Kind: PackageKindExternal, Path: importPath, ImportPath: importPath}
	}
}

func ImportPathFromPackageRef(module ModuleInfo, ref PackageRef) string {
	switch ref.Kind {
	case PackageKindLocal:
		if ref.Path == "" {
			return module.Path
		}
		return module.Path + "/" + ref.Path
	case PackageKindStd, PackageKindExternal:
		return ref.Path
	default:
		return ref.Path
	}
}

func IsStandardPackage(importPath string) bool {
	if importPath == "C" {
		return true
	}
	if cached, ok := stdPackageCache.Load(importPath); ok {
		return cached.(bool)
	}

	pkg, err := build.Default.Import(importPath, "", build.FindOnly)
	isStd := err == nil && pkg.Goroot
	actual, _ := stdPackageCache.LoadOrStore(importPath, isStd)
	return actual.(bool)
}

var stdPackageCache sync.Map
