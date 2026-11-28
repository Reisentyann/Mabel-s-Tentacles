// SPDX-License-Identifier: MIT
// SEARCH_DYNAMIC_ENV_2026 uses identifiers, never literal environment names.
package dynamicenv

import environment "os"

// ReadDynamicEnv receives the environment variable name from its caller.
func ReadDynamicEnv(variableName string) string {
	return environment.Getenv(variableName)
}

// ReadFromMap obtains the name from a map key at runtime.
func ReadFromMap(values map[string]string) string {
	for variableName := range values {
		return environment.Getenv(variableName)
	}
	return ""
}
