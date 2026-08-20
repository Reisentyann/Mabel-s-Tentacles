<template>
  <div class="app-layout">
    <header class="navbar">
      <router-link to="/files" class="brand">Mabel's Tentacles</router-link>
      <nav class="links">
        <router-link to="/files" class="link">Files</router-link>
        <router-link to="/dashboard" class="link">Activity</router-link>
      </nav>
      <div class="actions">
        <span v-if="authStore.username" class="username">
          {{ authStore.username }}
        </span>
        <button class="btn-ghost" @click="handleLogout">Logout</button>
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

const router = useRouter();
const authStore = useAuthStore();

const handleLogout = async () => {
  await authStore.logoutAction();
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
.actions {
  display: flex;
  align-items: center;
  gap: 1rem;
}
.username {
  color: var(--color-text-muted);
  font-size: 0.9rem;
}
.content {
  flex: 1;
  padding: 2rem;
  max-width: 1100px;
  width: 100%;
  margin: 0 auto;
}
</style>
