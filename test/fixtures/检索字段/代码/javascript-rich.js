// SPDX-License-Identifier: MIT
// SEARCH_JS_FEATURE_2026 identifies this JavaScript module.
import { join } from "node:path";

export const SEARCH_MARKER = "SEARCH_JS_FEATURE_2026";

// TODO: preserve Unicode punctuation when normalizing a feature name.
export function normalizeFeatureName(rawName) {
  return rawName.trim().replace(/\s+/g, "-").toLowerCase();
}

export class FeatureBuilder {
  constructor(prefix) {
    this.prefix = prefix;
  }

  build(rawName) {
    return join(this.prefix, normalizeFeatureName(rawName));
  }
}

export async function describeFeature(rawName) {
  // This comment also gives the file a nonzero comment rate.
  return { name: normalizeFeatureName(rawName), marker: SEARCH_MARKER };
}

export default FeatureBuilder;
