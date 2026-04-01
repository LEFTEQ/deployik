package projectconfig

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

const (
	FrameworkNextJS = "nextjs"
	FrameworkVite   = "vite"
	FrameworkAstro  = "astro"
	FrameworkStatic = "static"
)

const (
	RuntimeNextJSStandalone = "nextjs-standalone"
	RuntimeStatic           = "static"
	defaultNodeVersion      = "22"
)

type Settings struct {
	Framework       string
	Runtime         string
	RootDirectory   string
	OutputDirectory string
	InstallCommand  string
	BuildCommand    string
	NodeVersion     string
}

func ApplyProjectDefaults(project *db.Project) error {
	settings, err := Resolve(project)
	if err != nil {
		return err
	}

	project.Framework = settings.Framework
	project.RootDirectory = settings.RootDirectory
	project.OutputDirectory = settings.OutputDirectory
	project.InstallCommand = settings.InstallCommand
	project.BuildCommand = settings.BuildCommand
	project.NodeVersion = settings.NodeVersion
	return nil
}

func Resolve(project *db.Project) (Settings, error) {
	framework := NormalizeFramework(project.Framework)

	rootDirectory, err := NormalizeProjectPath(project.RootDirectory, true)
	if err != nil {
		return Settings{}, fmt.Errorf("root_directory: %w", err)
	}

	outputDirectory := strings.TrimSpace(project.OutputDirectory)
	if outputDirectory == "" {
		outputDirectory = DefaultOutputDirectory(framework)
	}
	outputDirectory, err = NormalizeProjectPath(outputDirectory, false)
	if err != nil {
		return Settings{}, fmt.Errorf("output_directory: %w", err)
	}

	installCommand := strings.TrimSpace(project.InstallCommand)
	if installCommand == "" {
		installCommand = DefaultInstallCommand(framework)
	}

	buildCommand := strings.TrimSpace(project.BuildCommand)
	if buildCommand == "" {
		buildCommand = DefaultBuildCommand(framework)
	}

	nodeVersion := strings.TrimSpace(project.NodeVersion)
	if nodeVersion == "" {
		nodeVersion = defaultNodeVersion
	}

	return Settings{
		Framework:       framework,
		Runtime:         RuntimeForFramework(framework),
		RootDirectory:   rootDirectory,
		OutputDirectory: outputDirectory,
		InstallCommand:  installCommand,
		BuildCommand:    buildCommand,
		NodeVersion:     nodeVersion,
	}, nil
}

func NormalizeFramework(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", FrameworkNextJS:
		return FrameworkNextJS
	case FrameworkVite:
		return FrameworkVite
	case FrameworkAstro:
		return FrameworkAstro
	case FrameworkStatic:
		return FrameworkStatic
	default:
		return FrameworkStatic
	}
}

func RuntimeForFramework(framework string) string {
	if NormalizeFramework(framework) == FrameworkNextJS {
		return RuntimeNextJSStandalone
	}
	return RuntimeStatic
}

func DefaultInstallCommand(framework string) string {
	return "bun install"
}

func DefaultBuildCommand(framework string) string {
	switch NormalizeFramework(framework) {
	case FrameworkNextJS, FrameworkVite, FrameworkAstro, FrameworkStatic:
		return "bun run build"
	default:
		return "bun run build"
	}
}

func DefaultOutputDirectory(framework string) string {
	if NormalizeFramework(framework) == FrameworkNextJS {
		return ".next"
	}
	return "dist"
}

func DefaultNodeVersion() string {
	return defaultNodeVersion
}

func NormalizeProjectPath(value string, allowEmpty bool) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "." {
		if allowEmpty {
			return "", nil
		}
		return "", fmt.Errorf("path cannot be empty")
	}

	if filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("path must be relative")
	}

	cleaned := filepath.Clean(trimmed)
	if cleaned == "." {
		if allowEmpty {
			return "", nil
		}
		return "", fmt.Errorf("path cannot be empty")
	}

	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path cannot escape the repository")
	}

	return filepath.ToSlash(cleaned), nil
}
