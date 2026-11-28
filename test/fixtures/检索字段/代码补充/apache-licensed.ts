// SPDX-License-Identifier: Apache-2.0
// SEARCH_CODE_EXTRA_APACHE_2026
import { join } from "node:path";

// TODO: validate the Apache fixture.
export function apacheLabel(name: string): string {
  return join("fixtures", name);
}
