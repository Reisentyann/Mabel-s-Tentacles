import api from './index';

export const getFiles = () => api.get('/files/');
export const searchFiles = (params) => api.get('/files/search', { params });
export const getFileMetadata = (path) => api.get('/files/metadata', { params: { path } });
export const describeFile = (data) => api.put('/files/metadata', data);
export const copyFile = (data) => api.post('/files/copy', data);

// 逻辑键改（文件管理域 2026-09-08）：与 MCP move_file 同一正主
export const moveFile = (data) => api.post('/files/move', data);

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
