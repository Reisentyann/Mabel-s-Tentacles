import { createRouter, createWebHistory } from 'vue-router';
import AppLayout from '../components/AppLayout.vue';
import { useAuthStore } from '../stores/auth';

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/LoginView.vue'),
    },
    {
      path: '/register',
      name: 'register',
      component: () => import('../views/RegisterView.vue'),
    },
    {
      path: '/',
      component: AppLayout,
      children: [
        { path: '', redirect: '/files' },
        {
          path: 'files',
          name: 'files',
          component: () => import('../views/FilesView.vue'),
        },
        {
          path: 'dashboard',
          name: 'dashboard',
          component: () => import('../views/DashboardView.vue'),
        },
      ],
    },
  ],
});

// 导航守卫（权限批次 2026-09-06）：登录/注册公开，其余页面未登录跳 /login。
// 深层防线在后端（require_auth=true），前端守卫只负责体验不闪白。
router.beforeEach((to) => {
  const auth = useAuthStore();
  const publicPages = ['login', 'register'];
  if (publicPages.includes(to.name)) {
    return true;
  }
  if (!auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } };
  }
  return true;
});

export default router;
