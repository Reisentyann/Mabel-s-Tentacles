# Agent Frontend Service

## 1. 简介 (Overview)
本项目是一个基于 Vue 3 + Vite 构建的现代化单页应用 (SPA)。它提供直观的用户交互界面，包含用户注册、登录、控制台 (Dashboard) 以及指令管理等功能，与后端 FastAPI 服务通过 REST API 进行紧密交互。

## 2. 核心技术栈
- **核心框架**: Vue 3 (使用 Composition API)
- **构建工具**: Vite
- **状态管理**: Pinia
- **路由管理**: Vue Router
- **网络请求**: Axios
- **部署环境**: Nginx (Docker 环境中提供静态资源与代理)

## 3. 目录结构
- `src/api/`: 封装对后端接口的网络请求，包含 Axios 实例拦截器和请求重试机制。
- `src/stores/`: Pinia 状态管理模块（如 `auth.js` 负责维护 Token 及用户登录状态）。
- `src/router/`: 页面路由映射及鉴权守卫。
- `src/views/`: 页面级组件 (`LoginView.vue`, `RegisterView.vue`, `DashboardView.vue` 等)。
- `src/App.vue` & `src/main.js`: 前端根组件与应用挂载点。
- `vite.config.js`: Vite 构建和开发代理配置。
- `nginx.conf`: 生产环境部署时 Nginx 的服务器及路由代理配置。
- `Dockerfile`: 用于构建静态文件并打包到 Nginx 镜像。

## 4. 架构优化建议 (Optimization Analysis)
为了提升用户体验及工程化水平，建议实施以下优化：
1. **环境变量管理改造**: 当前 Axios 的 `baseURL` 存在硬编码（如 `http://localhost:18000/api`），严重阻碍了环境切换。应当在根目录引入 `.env.development` 和 `.env.production`，使用 `import.meta.env.VITE_API_URL` 动态注入 API 地址。
2. **Nginx 配置与性能优化**:
   - `nginx.conf` 中应开启 `gzip` 或 `brotli` 压缩以减小静态资源体积。
   - 对 JS、CSS 静态文件增加合理的 `Cache-Control` 长缓存头。
   - 配置严格的单页应用 fallback 机制：`try_files $uri $uri/ /index.html;` 以防止路由刷新时 404。
3. **全局交互反馈**: 补充全局的请求 Loading 状态和统一的 UI Toast 提示（成功/失败提示）。当前接口请求如果处于 Pending 或报 Network Error，界面反馈较为生硬，不利于用户排查问题。
4. **路由鉴权与无感刷新**: 完善 Vue Router 的 `beforeEach` 守卫，限制未登录状态下对 Dashboard 的访问。在 Axios 拦截器中，加入对 401 状态码的拦截与 Token 无感刷新机制 (Refresh Token)。
