package projectconfig

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

const (
	FrameworkNextJS  = "nextjs"
	FrameworkVite    = "vite"
	FrameworkAstro   = "astro"
	FrameworkStatic  = "static"
	FrameworkNodeAPI = "node-api"
)

const (
	PackageManagerAuto = "auto"
	PackageManagerBun  = "bun"
	PackageManagerPnpm = "pnpm"
	PackageManagerNpm  = "npm"
	PackageManagerYarn = "yarn"
)

const (
	RuntimeNextJSStandalone = "nextjs-standalone"
	RuntimeStatic           = "static"
	RuntimeNodeAPI          = "node-api"
	defaultNodeVersion      = "22"
)

type Settings struct {
	Framework       string
	PackageManager  string
	Runtime         string
	RootDirectory   string
	OutputDirectory string
	InstallCommand  string
	BuildCommand    string
	NodeVersion     string
	StartCommand    string
	HealthPath      string
}

func ApplyProjectDefaults(project *db.Project) error {
	settings, err := Resolve(project)
	if err != nil {
		return err
	}

	project.Framework = settings.Framework
	project.PackageManager = settings.PackageManager
	project.RootDirectory = settings.RootDirectory
	project.OutputDirectory = settings.OutputDirectory
	project.InstallCommand = settings.InstallCommand
	project.BuildCommand = settings.BuildCommand
	project.NodeVersion = settings.NodeVersion
	project.StartCommand = settings.StartCommand
	project.HealthPath = settings.HealthPath
	return nil
}

func Resolve(project *db.Project) (Settings, error) {
	framework := NormalizeFramework(project.Framework)
	packageManager := NormalizePackageManager(project.PackageManager)

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
	if installCommand == "" || IsKnownInstallDefault(installCommand) {
		installCommand = DefaultInstallCommand(packageManager)
	}

	buildCommand := strings.TrimSpace(project.BuildCommand)
	if buildCommand == "" || IsKnownBuildDefault(buildCommand) {
		buildCommand = DefaultBuildCommand(packageManager)
	}

	nodeVersion := strings.TrimSpace(project.NodeVersion)
	if nodeVersion == "" {
		nodeVersion = defaultNodeVersion
	}

	runtime := RuntimeForFramework(framework)

	startCommand := strings.TrimSpace(project.StartCommand)
	if startCommand == "" {
		startCommand = DefaultStartCommand(runtime)
	}

	healthPath := strings.TrimSpace(project.HealthPath)
	if healthPath == "" {
		healthPath = DefaultHealthPath(runtime)
	}

	return Settings{
		Framework:       framework,
		PackageManager:  packageManager,
		Runtime:         runtime,
		RootDirectory:   rootDirectory,
		OutputDirectory: outputDirectory,
		InstallCommand:  installCommand,
		BuildCommand:    buildCommand,
		NodeVersion:     nodeVersion,
		StartCommand:    startCommand,
		HealthPath:      healthPath,
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
	case FrameworkNodeAPI:
		return FrameworkNodeAPI
	default:
		return FrameworkStatic
	}
}

func RuntimeForFramework(framework string) string {
	switch NormalizeFramework(framework) {
	case FrameworkNextJS:
		return RuntimeNextJSStandalone
	case FrameworkNodeAPI:
		return RuntimeNodeAPI
	default:
		return RuntimeStatic
	}
}

func NormalizePackageManager(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", PackageManagerAuto:
		return PackageManagerAuto
	case PackageManagerBun:
		return PackageManagerBun
	case PackageManagerPnpm:
		return PackageManagerPnpm
	case PackageManagerNpm:
		return PackageManagerNpm
	case PackageManagerYarn:
		return PackageManagerYarn
	default:
		return PackageManagerAuto
	}
}

func DefaultInstallCommand(packageManager string) string {
	switch NormalizePackageManager(packageManager) {
	case PackageManagerPnpm:
		return "pnpm install --frozen-lockfile"
	case PackageManagerNpm:
		return "npm ci"
	case PackageManagerYarn:
		return "yarn install --frozen-lockfile"
	case PackageManagerAuto, PackageManagerBun:
		fallthrough
	default:
		return "bun install --frozen-lockfile"
	}
}

func DefaultBuildCommand(packageManager string) string {
	switch NormalizePackageManager(packageManager) {
	case PackageManagerPnpm:
		return "pnpm run build"
	case PackageManagerNpm:
		return "npm run build"
	case PackageManagerYarn:
		return "yarn build"
	case PackageManagerAuto, PackageManagerBun:
		fallthrough
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

// DefaultStartCommand returns the runtime's default container start command.
// Only the node-api runtime has a non-empty default; the Next.js standalone
// and static runtimes bake their CMD into the generated Dockerfile directly.
func DefaultStartCommand(runtime string) string {
	if runtime == RuntimeNodeAPI {
		return "node dist/main.js"
	}
	return ""
}

// DefaultHealthPath returns the HTTP path the runtime's HEALTHCHECK probes.
// node-api defaults to /health (NestJS / Hono / Fastify convention); static
// and Next.js standalone probe the root document.
func DefaultHealthPath(runtime string) string {
	if runtime == RuntimeNodeAPI {
		return "/health"
	}
	return "/"
}

func DefaultNodeVersion() string {
	return defaultNodeVersion
}

func DefaultPackageManager() string {
	return PackageManagerAuto
}

// IsKnownInstallDefault returns true if the command matches any package
// manager's default install command.
func IsKnownInstallDefault(command string) bool {
	trimmed := strings.TrimSpace(command)
	for _, pm := range []string{PackageManagerAuto, PackageManagerBun, PackageManagerPnpm, PackageManagerNpm, PackageManagerYarn} {
		if trimmed == DefaultInstallCommand(pm) {
			return true
		}
	}
	return false
}

// IsKnownBuildDefault returns true if the command matches any package
// manager's default build command.
func IsKnownBuildDefault(command string) bool {
	trimmed := strings.TrimSpace(command)
	for _, pm := range []string{PackageManagerAuto, PackageManagerBun, PackageManagerPnpm, PackageManagerNpm, PackageManagerYarn} {
		if trimmed == DefaultBuildCommand(pm) {
			return true
		}
	}
	return false
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
