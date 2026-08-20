import api from './index';

export const getOperations = (page = 1, size = 20) =>
  api.get('/operations/', { params: { page, size } });
