<template>
  <div class="login-page">
    <form class="login-card" @submit.prevent="handleLogin">
      <h1>Mabel's Tentacles</h1>
      <p class="subtitle">请使用管理员账户登录</p>

      <label class="field">
        <span>Username</span>
        <input v-model="username" type="text" autocomplete="username" required />
      </label>

      <label class="field">
        <span>Password</span>
        <input
          v-model="password"
          type="password"
          autocomplete="current-password"
          required
        />
      </label>

      <button class="btn submit" type="submit" :disabled="loading">
        {{ loading ? "Signing in..." : "Login" }}
      </button>

      <p v-if="error" class="error">{{ error }}</p>
    </form>
  </div>
</template>

<script setup>
import { ref } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "../stores/auth";

const username = ref("");
const password = ref("");
const loading = ref(false);
const error = ref("");
const router = useRouter();
const authStore = useAuthStore();

const handleLogin = async () => {
  loading.value = true;
  error.value = "";
  try {
    await authStore.loginAction({
      username: username.value,
      password: password.value,
    });
    router.push("/files");
  } catch (err) {
    error.value = err.response?.data?.detail || "Login failed";
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
}
.login-card {
  width: 100%;
  max-width: 380px;
  background-color: var(--color-surface);
  padding: 2.5rem;
  border-radius: 12px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.08);
}
.login-card h1 {
  font-size: 1.4rem;
  text-align: center;
}
.subtitle {
  text-align: center;
  color: var(--color-text-muted);
  margin: 0 0 1.5rem;
  font-size: 0.9rem;
}
.field {
  display: block;
  margin-bottom: 1rem;
}
.field span {
  display: block;
  margin-bottom: 0.4rem;
  font-size: 0.85rem;
  color: var(--color-text-muted);
}
.field input {
  width: 100%;
  padding: 0.6rem;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  font-size: 1rem;
}
.field input:focus {
  outline: none;
  border-color: var(--color-primary);
}
.submit {
  width: 100%;
  margin-top: 0.5rem;
}
</style>
