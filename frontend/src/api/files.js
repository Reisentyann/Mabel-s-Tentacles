import api from './index';

export const getFiles = () => api.get('/files/');
export const searchFiles = (params) => api.get('/files/search', { params });
export const getFileMetadata = (path) => api.get('/files/metadata', { params: { path } });

// 索引机字段目录（检索契约的机器面，2026-09-10）：fields 各带 kind/values/
// min-max + desc（含义）+ bench（分档）；顶层 guide = 口径规则 + 速查表。
// 条件构造器的数据源，会话内缓存即可（动态但基本稳定）。
export const getIndexFields = (prefix) =>
  api.get('/index/fields', { params: prefix ? { prefix } : {} });
export const describeFile = (data) => api.put('/files/metadata', data);
export const copyFile = (data) => api.post('/files/copy', data);

// 逻辑键改（文件管理域 2026-09-08）：与 MCP move_file 同一正主
export const moveFile = (data) => api.post('/files/move', data);

// 软删除文件
export const deleteFile = (path) => api.post('/files/delete', { path });

// 获取 24 小时极简分享短链 (/d/{code})
export const getShareLink = (path) => api.post('/files/share', { path });

// 重分析 / 回填（管理台维护动作）
export const analyzeFile = (path) => api.post('/files/analyze', { path });
export const backfillFiles = () => api.post('/files/backfill');

// blob 下载（权限批次 2026-09-06）：走 axios 自动带 JWT，取代裸链接的
// VITE_ACCESS_TOKEN 构建期注入方案（那套在 token 为空时等于无凭证裸奔）
export const downloadFileBlob = (path) =>
  api.get('/files/download', { params: { path }, responseType: 'blob' });

// zip 打包下载
export const downloadZipBlob = (paths) =>
  api.post('/files/download-zip', { paths }, { responseType: 'blob' });
