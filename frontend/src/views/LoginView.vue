<template>
  <div class="login-page">
    <form class="login-card" @submit.prevent="handleLogin">
      <div class="brand">
        <span class="mark">🐙</span>
        <h1>Mabel's Tentacles</h1>
      </div>
      <p class="subtitle">触手书房 · 管理员入口</p>

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
        {{ loading ? '进入书房…' : '进入管理台' }}
      </button>

      <p v-if="error" class="error">{{ error }}</p>

      <p class="hint">本系统是 MCP 服务器，对接自有 agent；管理面仅限管理员（.env 种子账号），不开放注册。</p>
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
    error.value = e?.response?.data?.error || '登录失败（账号或密码不对）';
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
  background:
    radial-gradient(ellipse at 30% 20%, #241f31 0%, transparent 55%),
    radial-gradient(ellipse at 75% 80%, #1f2a2e 0%, transparent 50%),
    var(--mabel-bg);
}
.login-card {
  width: 340px;
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
  font-size: 1.6rem;
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
  color: #16121f;
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
