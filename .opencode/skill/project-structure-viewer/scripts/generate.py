#!/usr/bin/env python3
"""
generate.py — Project Structure Viewer Generator
==================================================
Scans a project directory and produces a self-contained interactive HTML file
that renders the complete file tree as a left-to-right horizontal diagram.

Usage: python3 scripts/generate.py <scanPath> <linkRoot> <outputDir>

Arguments:
  scanPath   — path to scan the filesystem
  linkRoot   — host path used for file:// links in the generated HTML
  outputDir  — directory where structure.html will be written
"""

import os, sys, json, fnmatch, html as html_mod

IGNORE = [
    'node_modules', '.git', '__pycache__', '.pnpm',
    '*.pyc', '*.pyo', '.DS_Store', 'Thumbs.db',
    '.idea', '.vscode', '*.swp', '*.swo', '*~',
    '.env', '.turbo', 'coverage', '.nyc_output',
    '.pytest_cache', '.mypy_cache', '.tox', '.ruff_cache',
    '.next', '.nuxt', '*.egg-info', '.terraform', '*.log', '.cache',
    '.venv', 'venv', 'dist', 'build', 'target', 'vendor',
    '.gradle', '.dart_tool', '.serverless', '.aws-sam',
    '.build', 'DerivedData', 'Pods',
]


def ok(name):
    for pat in IGNORE:
        if fnmatch.fnmatch(name, pat):
            return False
    return True


def build_tree(directory, root_dir):
    out = []
    try:
        entries = sorted(os.listdir(directory), key=lambda x: (
            not os.path.isdir(os.path.join(directory, x)), x.lower()
        ))
    except (PermissionError, OSError):
        return out

    for entry in entries:
        if not ok(entry):
            continue
        full = os.path.join(directory, entry)
        rel = os.path.relpath(full, root_dir)
        try:
            if os.path.isdir(full):
                children = build_tree(full, root_dir)
                out.append({"n": entry, "t": "d", "p": rel, "c": children})
            elif os.path.isfile(full):
                sz = os.path.getsize(full)
                out.append({"n": entry, "t": "f", "p": rel, "s": sz})
        except OSError:
            continue
    return out


def flow_set(tree):
    """Auto-detect files that are likely useful starting points."""
    files = set()
    root_files = {
        "readme.md", "readme.zh.md", "readme.rst", "readme.adoc",
        "skill.md", "license",
        "package.json", "pnpm-workspace.yaml", "turbo.json",
        "pyproject.toml", "requirements.txt", "setup.py", "setup.cfg",
        "go.mod", "cargo.toml", "pom.xml", "build.gradle", "settings.gradle",
        "package.swift", "pubspec.yaml", "composer.json", "gemfile",
        "makefile", "cmakelists.txt", "dockerfile", "docker-compose.yml",
        "compose.yml", "compose.yaml", "agents.md", "claude.md",
        "codex.md", "project_guide.md",
    }
    entry_names = {
        "main.py", "main.go", "main.rs", "main.swift", "main.kt", "main.java",
        "main.ts", "main.tsx", "main.js", "main.jsx", "index.ts", "index.tsx",
        "index.js", "index.jsx", "app.py", "app.ts", "app.js", "server.py",
        "server.ts", "server.js", "cli.py", "cli.ts", "__main__.py",
        "manage.py", "wsgi.py", "asgi.py", "train.py", "inference.py",
        "predict.py", "pipeline.py", "notebook.ipynb",
    }
    key_names = {
        "routes.py", "routes.ts", "router.py", "router.ts", "router.tsx",
        "middleware.py", "middleware.ts", "context.py", "context.ts",
        "schema.py", "schema.ts", "schema.prisma", "schema.graphql",
        "openapi.yaml", "openapi.yml", "openapi.json", "dockerfile",
        "terraform.tf", "main.tf", "values.yaml", "chart.yaml",
        "conftest.py", "pytest.ini", "tox.ini", "noxfile.py",
    }
    key_dirs = {
        ".github", "src", "app", "lib", "cmd", "internal", "pkg", "crates",
        "packages", "apps", "services", "api", "cli", "server", "client",
        "core", "domain", "models", "schemas", "migrations", "notebooks",
        "infra", "deploy", "k8s", "helm", "docs", "tests", "test",
        "scripts", "references", "agents",
    }

    def walk(nodes):
        for nd in nodes:
            p = nd["p"]
            name = nd["n"]
            lower_name = name.lower()
            lower_path = p.lower()
            parts = set(lower_path.split("/"))
            if nd["t"] == "f":
                if "/" not in p and lower_name in root_files:
                    files.add(p)
                if lower_name in entry_names or lower_name in key_names:
                    files.add(p)
                if lower_path.startswith(".github/workflows/") and lower_name.endswith((".yml", ".yaml")):
                    files.add(p)
                if parts & key_dirs:
                    if lower_name in key_names or lower_name.endswith((
                        ".proto", ".graphql", ".graphqls", ".sql", ".tf",
                        ".ipynb", ".md", ".rst", ".yaml", ".yml",
                    )):
                        files.add(p)
                if "scripts" in parts and lower_name.endswith((".py", ".sh", ".ts", ".js", ".mjs")):
                    files.add(p)
                if any(seg in parts for seg in ("pages", "routes", "views", "screens")):
                    if not lower_name.endswith((
                        ".css", ".scss", ".less", ".test.ts", ".test.tsx",
                        ".spec.ts", ".spec.tsx",
                    )):
                        files.add(p)
            if nd["t"] == "d":
                walk(nd.get("c", []))
    walk(tree)
    return files


def script_json(value):
    """Serialize JSON safely for an inline <script> block."""
    return (json.dumps(value, ensure_ascii=False)
            .replace("<", "\\u003c")
            .replace(">", "\\u003e")
            .replace("&", "\\u0026")
            .replace("\u2028", "\\u2028")
            .replace("\u2029", "\\u2029"))


CSS = r'''
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#000;color:#ccc;overflow:hidden;height:100vh;width:100vw;display:flex;flex-direction:row}
.guide-panel{background:#0a0a0a;border-right:1px solid #222;width:340px;min-width:340px;height:100vh;overflow-y:auto;flex-shrink:0;transition:width .25s,min-width .25s}
.guide-panel.collapsed{width:48px;min-width:48px;overflow:hidden}
.guide-panel.collapsed .guide-body{display:none}
.guide-panel.collapsed .guide-header h2,.guide-panel.collapsed #guideHint{display:none}
.guide-panel.collapsed .guide-header .arrow{font-size:16px}
.guide-header{display:flex;align-items:center;gap:8px;padding:10px 12px;cursor:pointer;user-select:none;position:sticky;top:0;background:#0a0a0a;z-index:5}
.guide-header:hover{background:#111}
.guide-header .arrow{transition:transform .2s;font-size:11px;color:#666;flex-shrink:0}
.guide-header.collapsed .arrow{transform:rotate(180deg)}
.guide-header h2{font-size:13px;color:#e0e0e0;display:flex;align-items:center;gap:6px;overflow:hidden}
.guide-header h2 .badge{font-size:9px;background:#222;color:#999;padding:2px 6px;border-radius:8px;font-weight:400;white-space:nowrap}
.guide-body{padding:0 12px 12px;display:flex;flex-direction:column;gap:14px}
.flow-col h3{font-size:11px;color:#999;margin-bottom:6px;font-weight:500}
.flow-step{display:flex;gap:6px;padding:3px 0;font-size:10px;align-items:flex-start;line-height:1.4}
.flow-step .num{flex-shrink:0;width:16px;height:16px;border-radius:50%;display:flex;align-items:center;justify-content:center;font-size:8px;font-weight:700;color:#000;background:#555}
.flow-step .file{color:#aaa;cursor:pointer;font-family:monospace;font-size:9px;word-break:break-all}
.flow-step .file:hover{color:#fff;text-decoration:underline}
.flow-step .desc{color:#666;font-size:9px}
.flow-note{font-size:9px;color:#666;margin-top:6px;padding:8px 10px;background:#0d0d0d;border-radius:5px;line-height:1.6}
.flow-note b{color:#ccc}
.main-area{flex:1;display:flex;flex-direction:column;overflow:hidden}
.topbar{background:#0a0a0a;border-bottom:1px solid #222;padding:7px 16px;display:flex;align-items:center;gap:10px;z-index:200;flex-shrink:0}
.topbar h1{font-size:13px;color:#ddd;white-space:nowrap}
.topbar .spacer{flex:1}
.topbar .search-wrap{position:relative}
.topbar input{padding:5px 10px;border-radius:5px;border:1px solid #333;background:#000;color:#ccc;font-size:12px;outline:none;width:170px}
.topbar input:focus{border-color:#555}
.topbar button{padding:5px 10px;border-radius:5px;border:1px solid #333;background:#0a0a0a;color:#999;font-size:11px;cursor:pointer;white-space:nowrap}
.topbar button:hover{background:#1a1a1a;color:#ccc}
.topbar button.on{background:#1a1a1a;color:#fff;border-color:#555}
.sr-drop{position:absolute;top:100%;left:0;right:0;background:#111;border:1px solid #333;border-top:none;border-radius:0 0 5px 5px;max-height:240px;overflow-y:auto;z-index:300;display:none}
.sr-drop.show{display:block}
.sr-item{padding:5px 10px;font-size:11px;cursor:pointer;display:flex;align-items:center;gap:6px;border-bottom:1px solid #1a1a1a}
.sr-item:hover{background:#1a1a1a}
.sr-item .sr-name{color:#ccc;font-family:monospace;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.sr-item .sr-path{color:#555;font-size:9px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;margin-left:auto;max-width:50%}
.sr-empty{padding:12px;text-align:center;color:#555;font-size:12px}
.sr-more{padding:6px;text-align:center;color:#666;font-size:10px}
.canvas-wrap{flex:1;overflow:auto;position:relative;cursor:grab}
.canvas-wrap:active{cursor:grabbing}
.canvas{position:relative;transform-origin:0 0;padding:20px 40px 40px 20px}
svg.lns{position:absolute;top:0;left:0;pointer-events:none;z-index:1}
.node{position:absolute;display:flex;align-items:center;gap:4px;padding:5px 10px;border-radius:5px;font-size:11px;white-space:nowrap;cursor:pointer;z-index:10;border:1px solid transparent;user-select:none;width:200px}
.node:hover{z-index:20;border-color:#444}
.node.hl{border-color:#777!important;z-index:25}
.node.dim{opacity:.08;pointer-events:none}
.node.dir{background:#0a0a0a;border-color:#1a1a1a}
.node.dir .ni{color:#bbb;font-weight:500;max-width:140px;overflow:hidden;text-overflow:ellipsis}
.node.dir .ico{font-size:12px;flex-shrink:0}
.node.file{background:#050505;border-color:#141414}
.node.file .ni{color:#aaa;max-width:120px;overflow:hidden;text-overflow:ellipsis}
.node.file .sz{font-size:9px;color:#444;margin-left:auto}
.node.flow{border-color:#555!important;background:#0d0d0d!important}
.node.flow .ni{color:#ddd!important}
.node.root{background:#0d0d0d;border-color:#444;font-weight:600}
.node.root .ni{color:#ddd}
.tt{position:fixed;background:#111;border:1px solid #333;border-radius:6px;padding:6px 10px;font-size:11px;color:#ccc;pointer-events:none;z-index:500;display:none}
.tt .p{color:#666;font-size:10px}
.zm{position:fixed;bottom:14px;right:14px;background:#111;border:1px solid #333;border-radius:5px;padding:5px 10px;font-size:11px;color:#555;z-index:100}
'''

JS = r'''
const DATA = __DATA__;
const PROOT = __LINKROOT__;
const FLOW = new Set(__FLOW__);
const NW = 200, NH = 30, LG = 78, NG = 4;
let els = [], sc = 1, exp = new Set(), sq = '';

let LANG = 'zh';
const T = {
  zh: {
    guideTitle:'📖 阅读路线图',guideBadge:'从哪里开始看？',
    guideHint:'点击收起侧栏 · 灰框 = 关键文件',
    searchPlaceholder:'🔍 搜索文件...',
    btnExpand:'📂 全部展开',btnCollapse:'📁 全部收起',btnReset:'🔄 重置',
    noMatch:'无匹配文件',more:'...还有 ',more2:' 个匹配',
    tooltipClick:'点击在编辑器中打开',
  },
  en: {
    guideTitle:'📖 Reading Guide',guideBadge:'Where to start?',
    guideHint:'Click to collapse · Gray-bordered nodes = key flow files',
    searchPlaceholder:'🔍 Search files...',
    btnExpand:'📂 Expand All',btnCollapse:'📁 Collapse',btnReset:'🔄 Reset',
    noMatch:'No matching files',more:'...',more2:' more matches',
    tooltipClick:'Click to open in editor',
  }
};

// File role descriptions (keyed by broad project patterns)
function descFile(p, n, lang) {
  const nm = n.toLowerCase(); const pp = p.toLowerCase();
  const zh = {
    'readme.md':'项目入口文档 — 说明用途、安装和主要工作流',
    'readme.zh.md':'中文项目入口文档',
    'skill.md':'Agent Skill 入口 — 定义触发条件、工作流和资源导航',
    'license':'许可证 — 说明复用和分发条件',
    'package.json':'JavaScript/TypeScript 项目清单 — 依赖、脚本、元信息',
    'pyproject.toml':'Python 项目清单 — 构建系统、依赖和工具配置',
    'requirements.txt':'Python 依赖清单',
    'setup.py':'Python 包安装入口',
    'go.mod':'Go 模块清单 — 模块路径和依赖版本',
    'cargo.toml':'Rust 包清单 — crate 元信息、依赖和构建配置',
    'pom.xml':'Maven 项目清单 — Java/Kotlin 依赖和构建生命周期',
    'build.gradle':'Gradle 构建脚本',
    'package.swift':'Swift Package Manager 清单',
    'pubspec.yaml':'Dart/Flutter 项目清单',
    'composer.json':'PHP Composer 项目清单',
    'gemfile':'Ruby Bundler 依赖清单',
    'tsconfig.json':'TypeScript 编译配置',
    'dockerfile':'容器镜像 — 定义生产运行环境',
    'docker-compose.yml':'本地或多服务编排配置',
    'compose.yaml':'本地或多服务编排配置',
    '.env.example':'环境变量模板 — 列出需要配置的变量',
    'pnpm-workspace.yaml':'Monorepo 工作区定义 — 声明子包位置',
    '.gitignore':'Git 忽略规则',
    'makefile':'常用开发、构建和发布命令入口',
    'cmakelists.txt':'C/C++ CMake 构建配置',
    'openapi.yaml':'OpenAPI 接口契约',
    'openapi.yml':'OpenAPI 接口契约',
    'openapi.json':'OpenAPI 接口契约',
    'openai.yaml':'Agent UI 元数据 — 显示名称、简介和默认提示',
  };
  if (zh[nm]) return zh[nm];
  if (zh[pp]) return zh[pp];
  if (/(^|\/)(main|index|app|server|cli|manage|__main__|wsgi|asgi|train|predict|inference|pipeline)\.(py|go|rs|swift|kt|java|tsx?|jsx?|rb|php|exs?)$/.test(pp)) return '运行入口 — 启动应用、命令行、服务或核心任务';
  if (/^scripts\/.+\.(py|sh|ts|js|mjs)$/.test(pp)) return '辅助脚本 — 自动化生成、验证、迁移或维护任务';
  if (/(^|\/)index\.html$/.test(pp)) return '浏览器入口 HTML — 声明页面挂载点和资源加载';
  if (/\.github\/workflows\/.+\.ya?ml$/.test(pp)) return 'CI/CD 工作流 — 自动化测试、构建或发布';
  if (/docker|compose|nginx|caddy/i.test(pp)) return '运行环境配置 — 容器、代理或本地编排';
  if (/terraform|\.tf$|helm|k8s|kubernetes|deploy|infra/i.test(pp)) return '基础设施/部署配置 — 描述运行环境和发布资源';
  if (/router|routes?|controller|handler|endpoint/i.test(pp)) return '接口适配层 — 暴露路由、命令、事件或外部入口';
  if (/middleware|interceptor|filter/i.test(pp)) return '中间层 — 处理认证、日志、校验、权限或请求包装';
  if (/schema|model|entity|migration|\.sql$|\.prisma$|\.proto$|graphql/i.test(pp)) return '数据契约/模型 — 定义持久化结构、消息格式或接口类型';
  if (/context|provider|container|registry/i.test(pp)) return '上下文/装配层 — 连接依赖、配置或运行时服务';
  if (/auth|permission|policy|session/i.test(pp)) return '身份与权限相关逻辑';
  if (/(pages|views|screens|components|ui)\//.test(pp)) return '用户界面层 — 页面、视图或可复用 UI 单元';
  if (/(cmd|bin)\//.test(pp)) return '命令行入口或可执行命令';
  if (/(core|domain|internal|pkg|lib|src)\//.test(pp)) return '核心实现模块 — 项目的主要业务或库逻辑';
  if (/(notebooks?|experiments?)\//.test(pp)||/\.ipynb$/.test(pp)) return '探索/分析笔记本 — 实验、数据分析或模型验证';
  if (/(tests?|spec|fixtures?)\//.test(pp)||/\.(test|spec)\./.test(pp)||/conftest\.py$/.test(pp)) return '验证资产 — 测试、样例数据或测试夹具';
  if (/(docs?|examples?)\//.test(pp)||/\.(md|rst|adoc)$/i.test(pp)) return '文档/示例 — 解释概念、使用方式或设计背景';
  if (/\.(ya?ml|toml|json|ini|cfg|conf)$/i.test(pp)) return '配置文件 — 控制工具、运行时或项目约定';
  return '';
}

function descFileEn(p, n) {
  const nm = n.toLowerCase(); const pp = p.toLowerCase();
  const en = {
    'readme.md':'Project entry document — purpose, setup, and primary workflows',
    'readme.zh.md':'Chinese project entry document',
    'skill.md':'Agent Skill entry — triggers, workflow, and resource navigation',
    'license':'License — reuse and distribution terms',
    'package.json':'JavaScript/TypeScript manifest — dependencies, scripts, metadata',
    'pyproject.toml':'Python project manifest — build system, dependencies, tooling',
    'requirements.txt':'Python dependency list',
    'setup.py':'Python package installation entry',
    'go.mod':'Go module manifest — module path and dependency versions',
    'cargo.toml':'Rust package manifest — crate metadata, dependencies, build config',
    'pom.xml':'Maven manifest — Java/Kotlin dependencies and build lifecycle',
    'build.gradle':'Gradle build script',
    'package.swift':'Swift Package Manager manifest',
    'pubspec.yaml':'Dart/Flutter project manifest',
    'composer.json':'PHP Composer manifest',
    'gemfile':'Ruby Bundler dependency manifest',
    'tsconfig.json':'TypeScript compiler configuration',
    'dockerfile':'Container image — defines production runtime',
    'docker-compose.yml':'Local or multi-service orchestration config',
    'compose.yaml':'Local or multi-service orchestration config',
    '.env.example':'Environment template — lists required variables',
    'pnpm-workspace.yaml':'Monorepo workspace — declares sub-package locations',
    '.gitignore':'Git ignore rules',
    'makefile':'Common development, build, and release command entry',
    'cmakelists.txt':'C/C++ CMake build configuration',
    'openapi.yaml':'OpenAPI interface contract',
    'openapi.yml':'OpenAPI interface contract',
    'openapi.json':'OpenAPI interface contract',
    'openai.yaml':'Agent UI metadata — display name, summary, and default prompt',
  };
  if (en[nm]) return en[nm];
  if (en[pp]) return en[pp];
  if (/(^|\/)(main|index|app|server|cli|manage|__main__|wsgi|asgi|train|predict|inference|pipeline)\.(py|go|rs|swift|kt|java|tsx?|jsx?|rb|php|exs?)$/.test(pp)) return 'Runtime entry — starts an app, CLI, service, or core task';
  if (/^scripts\/.+\.(py|sh|ts|js|mjs)$/.test(pp)) return 'Helper script — automates generation, validation, migration, or maintenance tasks';
  if (/(^|\/)index\.html$/.test(pp)) return 'Browser entry HTML — declares mount point and loaded assets';
  if (/\.github\/workflows\/.+\.ya?ml$/.test(pp)) return 'CI/CD workflow — automates tests, builds, or releases';
  if (/docker|compose|nginx|caddy/i.test(pp)) return 'Runtime environment config — containers, proxy, or local orchestration';
  if (/terraform|\.tf$|helm|k8s|kubernetes|deploy|infra/i.test(pp)) return 'Infrastructure/deployment config — describes runtime resources and rollout';
  if (/router|routes?|controller|handler|endpoint/i.test(pp)) return 'Interface adapter — exposes routes, commands, events, or external entry points';
  if (/middleware|interceptor|filter/i.test(pp)) return 'Middleware layer — authentication, logging, validation, permissioning, or request wrapping';
  if (/schema|model|entity|migration|\.sql$|\.prisma$|\.proto$|graphql/i.test(pp)) return 'Data contract/model — persistence structure, message format, or interface type';
  if (/context|provider|container|registry/i.test(pp)) return 'Context/composition layer — wires dependencies, configuration, or runtime services';
  if (/auth|permission|policy|session/i.test(pp)) return 'Identity and permission logic';
  if (/(pages|views|screens|components|ui)\//.test(pp)) return 'User interface layer — pages, screens, views, or reusable UI units';
  if (/(cmd|bin)\//.test(pp)) return 'Command-line entry or executable command';
  if (/(core|domain|internal|pkg|lib|src)\//.test(pp)) return 'Core implementation module — main product, domain, or library logic';
  if (/(notebooks?|experiments?)\//.test(pp)||/\.ipynb$/.test(pp)) return 'Exploration/analysis notebook — experiment, data analysis, or model validation';
  if (/(tests?|spec|fixtures?)\//.test(pp)||/\.(test|spec)\./.test(pp)||/conftest\.py$/.test(pp)) return 'Validation asset — tests, sample data, or fixtures';
  if (/(docs?|examples?)\//.test(pp)||/\.(md|rst|adoc)$/i.test(pp)) return 'Documentation/example — concepts, usage, or design background';
  if (/\.(ya?ml|toml|json|ini|cfg|conf)$/i.test(pp)) return 'Configuration file — controls tools, runtime, or project conventions';
  return '';
}

// ══ PROJECT-SPECIFIC GUIDE — REPLACE THIS ENTIRE BLOCK ══
// Replace with phase-based hardcoded content using mkS() / mkE().
// Example: mkS(1,'path/to/file.ts','Description in Chinese')
const GUIDE = {
  zh: '<div class="flow-col"><h3>📋 关键文件</h3><div class="flow-note">Agent 尚未生成项目专属阅读路线图。请重新运行 skill，确保先阅读项目源代码再替换此占位符。阶段格式参考：<br><br>🔷 阶段一：项目概览 (N)<br>🟢 阶段二：前端启动链 (N)<br>🟠 阶段三：后端请求链 (N)<br>🟣 关键流转总结<br><br>每阶段列出文件路径及中文描述，100% 覆盖所有项目文件。</div></div>',
  en: '<div class="flow-col"><h3>📋 Key Files</h3><div class="flow-note">Agent has not generated a project-specific reading guide yet. Re-run the skill and ensure project source files are read before replacing this placeholder. Expected phase format:<br><br>🔷 Phase 1: Project Overview (N)<br>🟢 Phase 2: Frontend Chain (N)<br>🟠 Phase 3: Backend Chain (N)<br>🟣 Cross-cutting Flows<br><br>Each phase lists file paths with descriptions, covering 100% of project files.</div></div>'
};

function mkS(n,p,d){return '<div class="flow-step"><span class="num">'+n+'</span><div><span class="file" data-p="'+escAttr(p)+'" onclick="navTo(this.dataset.p)">'+esc2(p)+'</span><div class="desc">'+esc2(d)+'</div></div></div>';}
function mkE(n,p,d){return '<div class="flow-step"><span class="num">'+n+'</span><div><span class="file" data-p="'+escAttr(p)+'" onclick="navTo(this.dataset.p)">'+esc2(p)+'</span><div class="desc">'+esc2(d)+'</div></div></div>';}

function toggleLang() {
  LANG = LANG === 'zh' ? 'en' : 'zh';
  document.getElementById('btnLang').textContent = LANG === 'zh' ? 'EN' : '中文';
  applyLang();
}
function applyLang() {
  const t = T[LANG];
  document.getElementById('guideTitle').textContent = t.guideTitle;
  document.getElementById('guideBadge').textContent = t.guideBadge;
  document.getElementById('guideHint').textContent = t.guideHint;
  document.getElementById('q').placeholder = t.searchPlaceholder;
  document.getElementById('btnExpand').textContent = t.btnExpand;
  document.getElementById('btnCollapse').textContent = t.btnCollapse;
  document.getElementById('btnReset').textContent = t.btnReset;
  document.getElementById('guideBody').innerHTML = GUIDE[LANG];
  search();
}
function toggleGuide() {
  document.getElementById('guide').classList.toggle('collapsed');
  document.getElementById('guideHdr').classList.toggle('collapsed');
}
function navTo(p) {
  const parts = p.split('/');
  for (let i=0;i<parts.length-1;i++) exp.add(parts.slice(0,i+1).join('/'));
  render();
  setTimeout(()=>{const el=els.find(e=>e.dataset.path===p);if(el)el.scrollIntoView({behavior:'smooth',block:'center',inline:'center'});},80);
}
function layout(nodes,d,sy,pcy) {
  let y=sy;const out=[];
  for(const nd of nodes) {
    if(nd.t==='f'){out.push({n:nd,d,y,pcy});y+=NH+NG;}
    else{
      const kids=nd.c||[],open=exp.has(nd.p),cy=y+NH/2;
      out.push({n:nd,d,y,pcy,hk:kids.length>0,open});
      if(open&&kids.length>0){const cs=layout(kids,d+1,y+NH+NG,cy);out.push(...cs);y=cs[cs.length-1].y+NH+NG;}
      else y+=NH+NG;
    }
  }
  return out;
}
function ml(x1,y1,x2,y2,c,w){const l=document.createElementNS('http://www.w3.org/2000/svg','line');l.setAttribute('x1',x1);l.setAttribute('y1',y1);l.setAttribute('x2',x2);l.setAttribute('y2',y2);l.setAttribute('stroke',c);l.setAttribute('stroke-width',w);return l;}
function render() {
  const ca=document.getElementById('canvas'),sv=document.getElementById('svgl');
  els.forEach(e=>e.remove());els=[];sv.innerHTML='';
  if(exp.size===0)DATA.forEach(n=>{if(n.t==='d')exp.add(n.p);});
  const flat=layout(DATA,0,0,null);
  const maxD=flat.reduce((m,n)=>Math.max(m,n.d),0);
  const lastY=flat.length?flat[flat.length-1].y+NH+40:400;
  const tw=(maxD+2)*(NW+LG)+200;
  sv.setAttribute('width',tw);sv.setAttribute('height',lastY);
  sv.setAttribute('viewBox','0 0 '+tw+' '+lastY);
  const pgrp=new Map();
  for(const f of flat){if(f.pcy!==null&&f.pcy!==undefined){const k=f.d+'|'+f.pcy;if(!pgrp.has(k))pgrp.set(k,[]);pgrp.get(k).push(f);}}
  for(const[,g]of pgrp){if(g.length<2)continue;const d=g[0].d,x=d*(NW+LG)-5;const y0=g[0].y+NH/2,y1=g[g.length-1].y+NH/2;sv.appendChild(ml(x,y0,x,y1,'#222',1));for(const gi of g)sv.appendChild(ml(x,gi.y+NH/2,x+7,gi.y+NH/2,'#222',1));}
  for(const f of flat){
    const x=f.d*(NW+LG),y=f.y;
    if(f.pcy!==null&&f.pcy!==undefined){const px=(f.d-1)*(NW+LG)+NW,py=f.pcy,cx=x,cy=y+NH/2,mx=px+(cx-px)/2;sv.appendChild(ml(px,py,mx,py,'#1a1a1a',1));sv.appendChild(ml(mx,py,mx,cy,'#1a1a1a',1));sv.appendChild(ml(mx,cy,cx,cy,'#1a1a1a',1));}
    const el=document.createElement('div');
    let cls='node '+(f.n.t==='d'?'dir':'file');
    if(f.d===0&&f.n.t==='d')cls+=' root';
    if(FLOW.has(f.n.p))cls+=' flow';
    el.className=cls;el.style.left=x+'px';el.style.top=y+'px';el.dataset.path=f.n.p;el.dataset.type=f.n.t;
    if(f.n.t==='d'){el.innerHTML='<span class="ico">'+(f.open?'📂':'📁')+'</span><span class="ni">'+esc2(f.n.n)+'</span>';if((f.n.c||[]).length>0)el.addEventListener('click',e=>{e.stopPropagation();if(exp.has(f.n.p))exp.delete(f.n.p);else exp.add(f.n.p);render();});}
    else{el.innerHTML='<span class="ico">📄</span><span class="ni">'+esc2(f.n.n)+'</span>'+(f.n.s?'<span class="sz">'+fmt(f.n.s)+'</span>':'');el.addEventListener('click',e=>{e.stopPropagation();window.open(fileUrl(f.n.p),'_blank');});el.addEventListener('mouseenter',e=>showTT(e,f.n));el.addEventListener('mouseleave',hideTT);}
    ca.appendChild(el);els.push(el);
  }
  updateHL();
}
function buildIndex(nodes){const idx=[];for(const n of nodes){if(n.t==='f')idx.push({name:n.n,path:n.p,size:n.s});if(n.c)idx.push(...buildIndex(n.c));}return idx;}
const fileIndex=buildIndex(DATA);
function search(){sq=document.getElementById('q').value.toLowerCase().trim();updateHL();updateDropdown();}
function updateDropdown(){
  const dd=document.getElementById('srdrop'),t=T[LANG];
  if(!sq||sq.length<1){dd.classList.remove('show');dd.innerHTML='';return;}
  const matches=fileIndex.filter(f=>f.path.toLowerCase().includes(sq));
  const MAX=30;
  dd.innerHTML='';
  if(matches.length===0){
    const empty=document.createElement('div');
    empty.className='sr-empty';
    empty.textContent=t.noMatch;
    dd.appendChild(empty);
  }else{
    matches.slice(0,MAX).forEach(f=>{
      const item=document.createElement('div');
      item.className='sr-item';
      item.addEventListener('mousedown',e=>{e.preventDefault();navTo(f.path);dd.classList.remove('show');});
      const name=document.createElement('span');
      name.className='sr-name';
      name.textContent=f.name;
      const path=document.createElement('span');
      path.className='sr-path';
      path.textContent=f.path;
      item.appendChild(name);
      item.appendChild(path);
      dd.appendChild(item);
    });
    if(matches.length>MAX){
      const more=document.createElement('div');
      more.className='sr-more';
      more.textContent=t.more+(matches.length-MAX)+t.more2;
      dd.appendChild(more);
    }
  }
  dd.classList.add('show');
}
function updateHL(){if(!sq){els.forEach(e=>e.classList.remove('dim','hl'));return;}els.forEach(e=>{const p=(e.dataset.path||'').toLowerCase();if(p.includes(sq)){e.classList.remove('dim');e.classList.add('hl');}else{e.classList.add('dim');e.classList.remove('hl');}});}
function showTT(e,nd){const t=document.getElementById('tt');t.innerHTML='<div>'+esc2(nd.n)+'</div><div class="p">'+esc2(nd.p)+'</div>';t.style.display='block';t.style.left=(e.clientX+14)+'px';t.style.top=(e.clientY-8)+'px';}
function hideTT(){document.getElementById('tt').style.display='none';}
function expandAll(){function f(ns){for(const n of ns){if(n.t==='d'&&n.c&&n.c.length>0){exp.add(n.p);f(n.c);}}}f(DATA);render();}
function collapseAll(){exp.clear();DATA.forEach(n=>{if(n.t==='d')exp.add(n.p);});render();}
let px=0,py=0,pan=false,sx,sy,spx,spy;
const wr=document.getElementById('wrap'),ca=document.getElementById('canvas');
wr.addEventListener('wheel',e=>{e.preventDefault();const d=Math.min(Math.abs(e.deltaY),150)*0.00125;const ns=Math.min(3,Math.max(0.15,sc*(1+Math.sign(-e.deltaY)*d)));const r=wr.getBoundingClientRect();const mx=e.clientX-r.left,my=e.clientY-r.top;px=mx-(mx-px)*(ns/sc);py=my-(my-py)*(ns/sc);sc=ns;applyT();},{passive:false});
wr.addEventListener('mousedown',e=>{if(e.target===wr||e.target===ca||e.target.id==='svgl'){pan=true;sx=e.clientX;sy=e.clientY;spx=px;spy=py;}});
window.addEventListener('mousemove',e=>{if(!pan)return;px=spx+(e.clientX-sx)/sc;py=spy+(e.clientY-sy)/sc;applyT();});
window.addEventListener('mouseup',()=>{pan=false;});
function applyT(){ca.style.transform='translate('+px+'px,'+py+'px) scale('+sc+')';document.getElementById('zm').textContent=Math.round(sc*100)+'%';}
function resetView(){sc=1;px=0;py=0;applyT();document.getElementById('q').value='';sq='';document.getElementById('srdrop').classList.remove('show');expandAll();}
document.addEventListener('keydown',e=>{if((e.ctrlKey||e.metaKey)&&e.key==='f'){e.preventDefault();document.getElementById('q').focus();}if(e.key==='Escape'){document.getElementById('q').value='';sq='';updateHL();document.getElementById('srdrop').classList.remove('show');}if(e.key==='0'&&(e.ctrlKey||e.metaKey)){e.preventDefault();resetView();}});
function fmt(b){if(!b)return'';const u=['B','KB','MB','GB'];let s=b,i=0;while(s>=1024&&i<u.length-1){s/=1024;i++;}return i===0?s+' B':s.toFixed(1)+' '+u[i];}
function esc2(s){const d=document.createElement('div');d.textContent=s;return d.innerHTML;}
function escAttr(s){return esc2(s).replace(/"/g,'&quot;');}
function fileUrl(p){
  const root=PROOT.replace(/\/+$/,'');
  const full=root+'/'+p;
  return 'file://'+full.split('/').map((seg,i)=>{
    if(i===0)return seg;
    const enc=encodeURIComponent(seg);
    return /^[A-Za-z]%3A$/.test(enc)?seg:enc;
  }).join('/');
}
applyLang();render();
'''

HTML_TEMPLATE = r'''<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>__TITLE__ — Project Structure</title>
<style>__CSS__</style>
</head>
<body>
<div class="guide-panel" id="guide">
<div class="guide-header" id="guideHdr" onclick="toggleGuide()">
  <span class="arrow">▼</span>
  <h2><span id="guideTitle"></span> <span class="badge" id="guideBadge"></span></h2>
  <span style="flex:1"></span>
  <span style="font-size:10px;color:#555" id="guideHint"></span>
</div>
<div class="guide-body" id="guideBody"></div>
</div>
<div class="main-area">
<div class="topbar">
  <h1>📂 __TITLE__</h1>
  <span class="spacer"></span>
  <span class="search-wrap">
    <input type="text" id="q" placeholder="" oninput="search()" onfocus="search()" onblur="setTimeout(()=>srdrop.classList.remove('show'),200)">
    <div class="sr-drop" id="srdrop"></div>
  </span>
  <button onclick="expandAll()" id="btnExpand"></button>
  <button onclick="collapseAll()" id="btnCollapse"></button>
  <button onclick="resetView()" id="btnReset"></button>
  <button onclick="toggleLang()" class="on" id="btnLang">EN</button>
</div>
<div class="canvas-wrap" id="wrap">
  <div class="canvas" id="canvas"><svg class="lns" id="svgl"></svg></div>
  <div class="zm" id="zm">100%</div>
</div>
</div>
<div class="tt" id="tt"></div>
<script>__JS__</script>
</body>
</html>'''


def generate(scan_path, link_root, output_dir):
    project_name = os.path.basename(os.path.abspath(scan_path)) or 'project'

    print(f'Scanning: {scan_path}')
    tree = build_tree(scan_path, scan_path)
    fl = flow_set(tree)
    flow_json = script_json(sorted(fl))
    tree_json = script_json(tree)

    file_count = sum(1 for _ in _walk_files(tree))
    dir_count = sum(1 for _ in _walk_dirs(tree))
    print(f'{dir_count} dirs, {file_count} files')

    js = JS.replace('__DATA__', tree_json)
    js = js.replace('__LINKROOT__', script_json(link_root.rstrip('/')))
    js = js.replace('__FLOW__', flow_json)

    html = HTML_TEMPLATE.replace('__CSS__', CSS.strip())
    html = html.replace('__JS__', js.strip())
    html = html.replace('__TITLE__', html_mod.escape(project_name))

    os.makedirs(output_dir, exist_ok=True)
    out_path = os.path.join(output_dir, 'structure.html')
    with open(out_path, 'w', encoding='utf-8') as f:
        f.write(html)

    print(f'Done: {out_path}')
    return out_path


def _walk_files(nodes):
    for n in nodes:
        if n['t'] == 'f':
            yield n
        if n.get('c'):
            yield from _walk_files(n['c'])


def _walk_dirs(nodes):
    for n in nodes:
        if n['t'] == 'd':
            yield n
            if n.get('c'):
                yield from _walk_dirs(n['c'])


if __name__ == '__main__':
    if len(sys.argv) < 4:
        print(__doc__)
        sys.exit(1)
    generate(sys.argv[1], sys.argv[2], sys.argv[3])
