<template>
  <div class="app-layout">
    <header class="navbar">
      <router-link to="/files" class="brand">Mabel's Tentacles</router-link>
      <nav class="links">
        <router-link to="/files" class="link">Files</router-link>
        <router-link to="/dashboard" class="link">Activity</router-link>
      </nav>
      <div v-if="auth.username" class="user-box">
        <span class="username">{{ auth.username }}</span>
        <button class="btn-ghost logout" @click="handleLogout">登出</button>
      </div>
    </header>
    <main class="content">
      <router-view />
    </main>
  </div>
</template>

<script setup>
import { useRouter } from 'vue-router';
import { useAuthStore } from '../stores/auth';

const auth = useAuthStore();
const router = useRouter();

const handleLogout = async () => {
  await auth.logoutAction();
  router.push('/login');
};
</script>

<style scoped>
.app-layout {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}
.navbar {
  display: flex;
  align-items: center;
  gap: 2rem;
  padding: 0 2rem;
  height: 56px;
  background-color: var(--color-surface);
  border-bottom: 1px solid var(--color-border);
}
.brand {
  font-weight: 700;
  font-size: 1.05rem;
  color: var(--color-text);
  text-decoration: none;
}
.links {
  display: flex;
  gap: 1.25rem;
  flex: 1;
}
.link {
  color: var(--color-text-muted);
  text-decoration: none;
  padding: 0.25rem 0.5rem;
  border-radius: 6px;
  font-weight: 500;
}
.link:hover {
  color: var(--color-text);
  background-color: var(--color-bg);
}
.link.router-link-active {
  color: var(--color-primary);
}
.user-box {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}
.username {
  font-size: 0.9rem;
  color: var(--color-text-muted);
}
.logout {
  font-size: 0.85rem;
  padding: 0.25rem 0.75rem;
}
.content {
  flex: 1;
  padding: 2rem;
  max-width: 1100px;
  width: 100%;
  margin: 0 auto;
}
</style>
