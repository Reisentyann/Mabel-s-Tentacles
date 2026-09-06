<template>
  <div class="login-page">
    <form class="login-card" @submit.prevent="handleRegister">
      <h1>Mabel's Tentacles</h1>
      <p class="subtitle">注册一个账号（密码至少 8 位）</p>

      <label class="field">
        <span>Username</span>
        <input v-model="username" type="text" autocomplete="username" required />
      </label>

      <label class="field">
        <span>Password</span>
        <input
          v-model="password"
          type="password"
          autocomplete="new-password"
          minlength="8"
          required
        />
      </label>

      <label class="field">
        <span>Confirm Password</span>
        <input v-model="confirm" type="password" autocomplete="new-password" minlength="8" required />
      </label>

      <button class="btn submit" type="submit" :disabled="loading">
        {{ loading ? "Creating..." : "Register" }}
      </button>

      <p v-if="error" class="error">{{ error }}</p>

      <p class="alt">
        已有账号？
        <router-link to="/login">去登录</router-link>
      </p>
    </form>
  </div>
</template>

<script setup>
import { ref } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "../stores/auth";

const username = ref("");
const password = ref("");
const confirm = ref("");
const loading = ref(false);
const error = ref("");
const router = useRouter();
const authStore = useAuthStore();

const handleRegister = async () => {
  if (password.value !== confirm.value) {
    error.value = "两次输入的密码不一致";
    return;
  }
  loading.value = true;
  error.value = "";
  try {
    await authStore.registerAction({
      username: username.value,
      password: password.value,
    });
    router.push("/files");
  } catch (err) {
    error.value = err.response?.data?.detail || "注册失败";
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
.alt {
  text-align: center;
  margin: 1rem 0 0;
  font-size: 0.9rem;
  color: var(--color-text-muted);
}
</style>
