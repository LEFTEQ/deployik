package services

import "strings"

// MergeWithUserOverride composes the final runtime env-var slice for the app
// container. User-set keys (already in userVars, "KEY=VAL" shape) ALWAYS win
// over injected values — both for plain env and secrets. Injected keys not
// already present in userVars are appended.
//
// The returned slice has the user's vars first (preserving their relative
// order), followed by the injected non-conflicting keys in alphabetical order
// so the result is deterministic for tests + audit diffing.
func MergeWithUserOverride(userVars []string, inj EnvInjection) []string {
	seen := make(map[string]struct{}, len(userVars))
	for _, v := range userVars {
		if i := strings.IndexByte(v, '='); i > 0 {
			seen[v[:i]] = struct{}{}
		}
	}

	result := make([]string, 0, len(userVars)+len(inj.Env)+len(inj.Secrets))
	result = append(result, userVars...)

	// Walk injection in deterministic order so tests + audit logs are stable.
	for _, key := range sortedKeys(inj.Env) {
		if _, exists := seen[key]; exists {
			continue
		}
		result = append(result, key+"="+inj.Env[key])
	}
	for _, key := range sortedKeys(inj.Secrets) {
		if _, exists := seen[key]; exists {
			continue
		}
		result = append(result, key+"="+inj.Secrets[key])
	}
	return result
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// stdlib sort.Strings would do, but we keep zero external deps in this file.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}
