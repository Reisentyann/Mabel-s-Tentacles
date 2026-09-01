import { createRouter, createWebHistory } from 'vue-router';
import AppLayout from '../components/AppLayout.vue';

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      redirect: '/files',
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

export default router;
