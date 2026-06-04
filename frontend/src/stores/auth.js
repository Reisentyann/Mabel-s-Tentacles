import { defineStore } from 'pinia';
import { login, logout, register } from '../api/auth';

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('token') || null,
    refreshToken: localStorage.getItem('refresh_token') || null,
    user: JSON.parse(localStorage.getItem('user')) || null,
  }),
  getters: {
    isAuthenticated: (state) => !!state.token,
  },
  actions: {
    async loginAction(credentials) {
      const response = await login(credentials);
      this.setAuthData(response.data);
      return response;
    },
    async registerAction(data) {
      return await register(data);
    },
    async logoutAction() {
      try {
        if (this.refreshToken) {
          await logout(this.refreshToken);
        }
      } catch (error) {
        console.error('Logout failed:', error);
      } finally {
        this.clearAuthData();
      }
    },
    setAuthData(data) {
      this.token = data.access_token;
      this.refreshToken = data.refresh_token;
      
      // Basic decode of JWT for username if needed, or just rely on state
      // For now, storing a placeholder user
      this.user = { username: 'user' }; 
      
      localStorage.setItem('token', this.token);
      localStorage.setItem('refresh_token', this.refreshToken);
      localStorage.setItem('user', JSON.stringify(this.user));
    },
    clearAuthData() {
      this.token = null;
      this.refreshToken = null;
      this.user = null;
      localStorage.removeItem('token');
      localStorage.removeItem('refresh_token');
      localStorage.removeItem('user');
    }
  }
});