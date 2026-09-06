import api from './index';

export const login = (data) => api.post('/auth/login', data);

export const register = (data) => api.post('/auth/register', data);

export const logout = (refreshToken) =>
  api.post('/auth/logout', { refresh_token: refreshToken });
