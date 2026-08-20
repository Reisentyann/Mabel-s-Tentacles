import axios from 'axios';
import {
  getAccessToken,
  getRefreshToken,
  setTokens,
  clearTokens,
} from '../utils/token';

const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL || '/api',
  timeout: 15000,
});

api.interceptors.request.use((config) => {
  const token = getAccessToken();
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

async function refreshTokens(refreshToken) {
  const { data } = await axios.post(
    `${import.meta.env.VITE_API_URL || '/api'}/auth/refresh`,
    { refresh_token: refreshToken },
  );
  return data;
}

api.interceptors.response.use(
  (response) => response,
  async (error) => {
    const { config, response } = error;

    if (response?.status !== 401 || config._retry) {
      return Promise.reject(error);
    }

    const refreshToken = getRefreshToken();
    if (!refreshToken) {
      clearTokens();
      return Promise.reject(error);
    }

    config._retry = true;
    try {
      const data = await refreshTokens(refreshToken);
      setTokens(data.access_token, data.refresh_token);
      config.headers.Authorization = `Bearer ${data.access_token}`;
      return api(config);
    } catch (refreshError) {
      clearTokens();
      return Promise.reject(refreshError);
    }
  },
);

export default api;
