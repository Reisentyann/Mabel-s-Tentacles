import { createRouter, createWebHistory } from 'vue-router';
import AppLayout from '../components/AppLayout.vue';
import { useAuthStore } from '../stores/auth';

// 单页管理台架构（2026-09-08 前端重设计）：登录 + manage 一页
// （树/表/详情抽屉/统计全部整合）。files/dashboard 两页与 register 退役
// ——本系统是 MCP 服务器对接自有 agent，管理面是自用工具不是公众产品，
// 开放注册没有意义（管理员账号走 .env 种子）。
const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/LoginView.vue'),
    },
    {
      path: '/',
      component: AppLayout,
      children: [
        { path: '', redirect: '/manage' },
        {
          path: 'manage',
          name: 'manage',
          component: () => import('../views/ManageView.vue'),
        },
      ],
    },
  ],
});

// 路由前置守卫（权限批次 2026-09-06）：鉴权归后端（require_auth=true），
// 前端只做体验性拦截不越权。
router.beforeEach((to) => {
  const auth = useAuthStore();
  if (to.name === 'login') {
    return true;
  }
  if (!auth.token) {
    return { name: 'login' };
  }
  return true;
});

export default router;
