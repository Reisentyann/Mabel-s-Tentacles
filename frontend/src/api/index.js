import axios from 'axios';

const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL || 'http://localhost:18000/api',
  timeout: 10000,
});

api.interceptors.request.use(
  (config) => {
    console.log(`[DEBUG] API Request: ${config.method.toUpperCase()} ${config.baseURL || ''}${config.url}`, config.data);
    const token = localStorage.getItem('token');
    if (token) {
      config.headers['Authorization'] = `Bearer ${token}`;
    }
    return config;
  },
  (error) => {
    console.error('[DEBUG] API Request Interceptor Error:', error);
    return Promise.reject(error);
  }
);

api.interceptors.response.use(
  (response) => {
    console.log(`[DEBUG] API Response: ${response.status} from ${response.config.url}`, response.data);
    return response;
  },
  (error) => {
    console.error('[DEBUG] API Response Interceptor Error:', error);
    // Handle 401 Unauthorized
    if (error.response && error.response.status === 401) {
      console.log('[DEBUG] 401 Unauthorized, redirecting to login');
      // You can implement automatic token refresh here or logout
      localStorage.removeItem('token');
      window.location.href = '/login';
    }
    return Promise.reject(error);
  }
);

export default api;