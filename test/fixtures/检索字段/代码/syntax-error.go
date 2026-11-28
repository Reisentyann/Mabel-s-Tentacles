// SPDX-License-Identifier: MIT
// SEARCH_SYNTAX_ERROR_2026 is intentionally invalid; do not compile this file.
package searchsyntax

import environment "os"

// TODO: repair the missing closing parenthesis in the parameter list.
func BrokenLookup(value string {
	// FIXME: AST extraction must fail even though text facts remain available.
	return environment.Getenv(value)
}
