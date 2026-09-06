---
name: project-structure-viewer
description: Generate an interactive structure map for any codebase or project directory. Use when the user asks to understand, visualize, map, inspect, or get oriented in a repository, monorepo, library, CLI, app, data/ML project, infrastructure repo, documentation site, or mixed workspace.
license: MIT
compatibility: Requires Python 3.10+ to run the bundled generator script.
---

# Project Structure Viewer

Create a self-contained `structure.html` that shows a project's complete file tree as an interactive left-to-right map with search, pan/zoom, expand/collapse, clickable local file links, and an auto-detected key-file guide.

## Bundled Resources

- `scripts/generate.py` — scans a directory and writes `structure.html`.
- `references/project-taxonomy.md` — use when classifying unfamiliar, non-web, or mixed project types.

## Workflow

1. Resolve paths:
   - `scanPath`: the filesystem path to scan.
   - `linkRoot`: the local host path used for `file://` links. Usually the same as `scanPath`.
   - `outputDir`: where to write `structure.html`. Usually the project root.

2. Inspect before generating. Read enough real files to understand what kind of project this is. Start with root docs and manifests, then entry points, key configs, tests, automation, and domain modules. Use `references/project-taxonomy.md` when the project is not an obvious frontend/backend app or when it mixes ecosystems.

3. Build a phase map based on the observed project, not a fixed web-app template. Common phases include orientation/docs, dependency/build setup, runtime entry points, domain/core logic, interfaces/adapters, data/storage/contracts, automation/testing/release, infrastructure/deployment, and examples/assets. For large repeated directories, summarize the directory instead of reading every near-identical file.

4. **Copy, edit, run, delete — the agent does all of this automatically.**

```
# ① Copy the clean generate.py into the project (DO NOT edit the original)
SKILL_DIR = directory containing this SKILL.md
cp "$SKILL_DIR/scripts/generate.py" "<outputDir>/_gen.py"

# ② Edit <outputDir>/_gen.py:
#    Search for:  const GUIDE = {
#    There is a placeholder block with comment "REPLACE THIS ENTIRE BLOCK"
#    Replace it entirely with your phase-based GUIDE from step 3
#    Format:
#      zh: [ '<div class="flow-col"><h3>🔷 阶段一：项目概览 (5)</h3>', mkS(1,'pkg.json','...'), '</div>', ... ].join('')
#      en: [ ... same using mkE() ... ].join('')

# ③ Run the copy
python3 "<outputDir>/_gen.py" "<scanPath>" "<linkRoot>" "<outputDir>"

# ④ Delete the copy
rm "<outputDir>/_gen.py"
```

The original `scripts/generate.py` stays pristine. The agent handles all of this — the user never touches the script.

5. Validate the result:
   - Confirm `<outputDir>/structure.html` exists.
   - If possible, open or preview the HTML and check that the tree is non-empty.
   - Do not leave generated test files outside the requested output location unless needed for debugging.

6. Report back with:
   - The absolute path to `structure.html`.
   - A short orientation summary of the project phases you inferred.
   - Any caveats, such as skipped huge/generated directories or unreadable paths.

## Static Diagram Option

This skill's default output is an interactive HTML tree. If the user explicitly wants a polished static architecture/flow diagram as SVG or PNG, pair this with a diagram skill such as `fireworks-tech-graph` when available. Use the structure map and the phase map as input for that static diagram; do not replace the interactive tree unless the user asks for that instead.
