package build

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

func resolveDeploymentVariables(envVars, secrets []db.ProjectVariable) ([]EnvVar, []string) {
	var buildEnvVars []EnvVar
	runtimeEnvVars := make([]string, 0, len(envVars)+len(secrets))

	for _, variable := range envVars {
		if strings.HasPrefix(variable.Key, "NEXT_PUBLIC_") {
			buildEnvVars = append(buildEnvVars, EnvVar{Key: variable.Key, Value: variable.Value})
		}
		runtimeEnvVars = append(runtimeEnvVars, variable.Key+"="+variable.Value)
	}

	for _, variable := range secrets {
		runtimeEnvVars = append(runtimeEnvVars, variable.Key+"="+variable.Value)
	}

	return buildEnvVars, runtimeEnvVars
}

// resolveBuildSecrets returns the set of variables that should be exposed to
// the build via a BuildKit `--mount=type=secret` mount — i.e. everything the
// container gets at runtime, minus the NEXT_PUBLIC_* vars that are already
// baked as ENV in the builder stage.
//
// Exposing ordinary env vars here matches Vercel's semantics (build can read
// any env var) while keeping values out of image layers: they live only in the
// tmpfs mount for the duration of the RUN step that consumes them.
func resolveBuildSecrets(envVars, secrets []db.ProjectVariable) []EnvVar {
	out := make([]EnvVar, 0, len(envVars)+len(secrets))
	for _, v := range envVars {
		if strings.HasPrefix(v.Key, "NEXT_PUBLIC_") {
			continue
		}
		out = append(out, EnvVar{Key: v.Key, Value: v.Value})
	}
	for _, v := range secrets {
		out = append(out, EnvVar{Key: v.Key, Value: v.Value})
	}
	return out
}

// writeBuildSecretsFile serializes vars as a shell-sourceable env file
// (`KEY='value'` lines with single-quote escaping) into dir, returning the
// absolute path. Returns "" with no error when there is nothing to write.
//
// The file is created with 0600 permissions by os.CreateTemp and is expected
// to be removed by the caller after the build finishes.
func writeBuildSecretsFile(dir string, vars []EnvVar) (string, error) {
	if len(vars) == 0 {
		return "", nil
	}
	if dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", fmt.Errorf("create secrets dir: %w", err)
		}
	}
	f, err := os.CreateTemp(dir, "deployik-build-secrets-*.env")
	if err != nil {
		return "", fmt.Errorf("create secrets file: %w", err)
	}
	path := f.Name()

	var buf bytes.Buffer
	for _, v := range vars {
		buf.WriteString(v.Key)
		buf.WriteByte('=')
		buf.WriteString(shellSingleQuote(v.Value))
		buf.WriteByte('\n')
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		os.Remove(path)
		return "", fmt.Errorf("write secrets file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("close secrets file: %w", err)
	}
	return path, nil
}

// shellSingleQuote wraps s in single quotes, escaping embedded single quotes
// the classic POSIX way ('\'' ends the quoted string, inserts a literal quote,
// reopens). Safe to use with `sh -c`, `. file`, and BuildKit secret file
// sourcing on alpine/ash, bash, dash, and zsh.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
