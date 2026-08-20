import { defineStore } from 'pinia';
import { login, logout } from '../api/auth';
import {
  getAccessToken,
  getRefreshToken,
  setTokens,
  clearTokens,
} from '../utils/token';
import { decodeJwt } from '../utils/jwt';

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: getAccessToken(),
    refreshToken: getRefreshToken(),
  }),
  getters: {
    isAuthenticated: (state) => {
      if (!state.token) return false;
      const decoded = decodeJwt(state.token);
      return !!decoded?.exp && decoded.exp * 1000 > Date.now();
    },
    username: (state) => decodeJwt(state.token)?.sub || null,
  },
  actions: {
    async loginAction(credentials) {
      const { data } = await login(credentials);
      this.setAuthData(data);
      return data;
    },
    async logoutAction() {
      try {
        if (this.refreshToken) {
          await logout(this.refreshToken);
        }
      } catch {
        // ignore logout API errors
      } finally {
        this.clearAuthData();
      }
    },
    setAuthData(data) {
      this.token = data.access_token;
      this.refreshToken = data.refresh_token;
      setTokens(data.access_token, data.refresh_token);
    },
    clearAuthData() {
      this.token = null;
      this.refreshToken = null;
      clearTokens();
    },
  },
});
