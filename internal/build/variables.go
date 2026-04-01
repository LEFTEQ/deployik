package build

import (
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
