<template>
  <div class="login-page">
    <form class="login-card" @submit.prevent="handleLogin">
      <div class="brand">
        <span class="mark">M</span>
        <h1>Mabel's Tentacles</h1>
      </div>
      <p class="subtitle">管理控制台 · 登录</p>

      <label class="field">
        <span>账号</span>
        <input v-model="username" type="text" autocomplete="username" required />
      </label>

      <label class="field">
        <span>密码</span>
        <input
          v-model="password"
          type="password"
          autocomplete="current-password"
          required
        />
      </label>

      <button class="submit" type="submit" :disabled="loading">
        {{ loading ? '登录中…' : '登录' }}
      </button>

      <p v-if="error" class="error">{{ error }}</p>

      <p class="hint">请使用管理员账号登录。</p>
    </form>
  </div>
</template>

<script setup>
import { ref } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { useAuthStore } from '../stores/auth';

const username = ref('');
const password = ref('');
const loading = ref(false);
const error = ref('');
const router = useRouter();
const route = useRoute();
const authStore = useAuthStore();

const handleLogin = async () => {
  loading.value = true;
  error.value = '';
  try {
    await authStore.loginAction({
      username: username.value,
      password: password.value,
    });
    router.push(route.query.redirect || '/manage');
  } catch (e) {
    if (!e.response || e.response.status >= 500) {
      error.value = '后端服务连接失败，请确认 Go 后端（:8080）已启动';
    } else {
      error.value = e?.response?.data?.error || '登录失败（账号或密码不正确）';
    }
  } finally {
    loading.value = false;
  }
};
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
    background: var(--mabel-bg);
}
.login-card {
  width: min(400px, calc(100vw - 32px));
  padding: 32px 28px;
  background-color: var(--mabel-surface);
  border: 1px solid var(--mabel-border);
  border-radius: 14px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
}
.mark {
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border-radius: 6px;
  background: var(--el-color-primary);
  color: white;
  font-size: 1.1rem;
  font-weight: 600;
}
h1 {
  font-size: 1.15rem;
  margin: 0;
}
.subtitle {
  margin: -6px 0 6px;
  font-size: 0.82rem;
  color: var(--mabel-text-muted);
}
.field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 0.82rem;
  color: var(--mabel-text-muted);
}
.field input {
  padding: 9px 12px;
  border-radius: 8px;
  border: 1px solid var(--mabel-border);
  background-color: var(--mabel-surface-2);
  color: var(--mabel-text);
  outline: none;
}
.field input:focus {
  border-color: var(--el-color-primary);
}
.submit {
  padding: 10px;
  border: none;
  border-radius: 8px;
  background-color: var(--el-color-primary);
  color: #ffffff;
  font-weight: 700;
  cursor: pointer;
}
.submit:disabled {
  opacity: 0.6;
  cursor: wait;
}
.error {
  margin: 0;
  font-size: 0.8rem;
  color: #e8756a;
}
.hint {
  margin: 4px 0 0;
  font-size: 0.72rem;
  line-height: 1.5;
  color: var(--mabel-text-muted);
}
</style>
