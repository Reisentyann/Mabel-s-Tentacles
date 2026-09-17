<template>
  <div class="app-layout">
    <header class="navbar">
      <router-link to="/manage" class="brand">
        <span class="brand-mark">🐙</span>
        <span>Mabel's Tentacles</span>
        <span class="brand-sub">触手书房</span>
      </router-link>
      <nav class="nav-links">
        <router-link to="/manage" class="nav-link" active-class="active">
          <span>📁 文件工作区</span>
        </router-link>
        <router-link to="/operations" class="nav-link" active-class="active">
          <span>📜 调用日志</span>
        </router-link>
      </nav>
      <div class="flex-fill"></div>
      <div v-if="auth.username" class="user-box">
        <el-tag size="small" type="info" effect="plain">{{ auth.username }}</el-tag>
        <el-button size="small" text @click="handleLogout">登出</el-button>
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
  background-color: var(--mabel-bg);
}
.navbar {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0 16px;
  height: 56px;
  flex: none;
  background-color: var(--mabel-surface);
  border-bottom: 1px solid var(--mabel-border);
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  font-weight: 700;
  font-size: 1rem;
  color: var(--mabel-text);
  text-decoration: none;
}
.brand-mark {
  font-size: 1.2rem;
}
.brand-sub {
  font-size: 0.78rem;
  font-weight: 500;
  color: var(--mabel-text-muted);
}
.nav-links {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-left: 20px;
}
.nav-link {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 14px;
  border-radius: 6px;
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--mabel-text-muted);
  text-decoration: none;
  transition: all 0.2s;
}
.nav-link:hover {
  color: var(--mabel-text);
  background: var(--mabel-surface-2);
}
.nav-link.active {
  color: #fff;
  background: var(--el-color-primary-light-9);
  border: 1px solid var(--el-color-primary-light-8);
  box-shadow: 0 2px 8px rgba(184, 118, 217, 0.25);
}
.flex-fill {
  flex: 1;
}
.user-box {
  display: flex;
  align-items: center;
  gap: 8px;
}
.content {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
</style>
