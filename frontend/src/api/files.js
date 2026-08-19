import api from './index';

export const getFiles = () => {
  return api.get('/files/');
};

// Return the full download URL so it can be opened in a new tab or downloaded directly
export const getFileDownloadUrl = (filename) => {
  return `/api/files/download/${encodeURIComponent(filename)}`;
};

export const downloadFileDirectly = async (filename) => {
  return api.get(`/files/download/${encodeURIComponent(filename)}`, {
    responseType: 'blob'
  });
};
