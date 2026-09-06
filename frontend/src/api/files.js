import api from './index';

export const getFiles = () => api.get('/files/');
export const searchFiles = (params) => api.get('/files/search', { params });
export const getFileMetadata = (path) => api.get('/files/metadata', { params: { path } });
export const describeFile = (data) => api.put('/files/metadata', data);
export const copyFile = (data) => api.post('/files/copy', data);

// blob 下载（权限批次 2026-09-06）：走 axios 自动带 JWT，取代裸链接的
// VITE_ACCESS_TOKEN 构建期注入方案（那套在 token 为空时等于无凭证裸奔）
export const downloadFileBlob = (path) =>
  api.get('/files/download', { params: { path }, responseType: 'blob' });
