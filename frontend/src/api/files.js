import api from './index';

export const getFiles = () => api.get('/files/');
export const searchFiles = (params) => api.get('/files/search', { params });
export const getFileMetadata = (path) => api.get('/files/metadata', { params: { path } });
export const describeFile = (data) => api.put('/files/metadata', data);
export const copyFile = (data) => api.post('/files/copy', data);
