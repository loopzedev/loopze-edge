import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'

import { useAuthStore } from '@/stores/authStore'
import { basePath } from '@/runtime'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'FlowEditor',
    component: () => import('@/views/FlowEditor.vue'),
  },
  {
    path: '/settings',
    name: 'Settings',
    component: () => import('@/views/SettingsView.vue'),
  },
  {
    path: '/users',
    name: 'Users',
    component: () => import('@/views/UsersView.vue'),
    meta: { requiresAdmin: true },
  },
  {
    path: '/certs',
    name: 'Certificates',
    component: () => import('@/views/CertsView.vue'),
  },
  {
    path: '/design-preview',
    name: 'DesignPreview',
    component: () => import('@/views/DesignPreview.vue'),
  },
  {
    path: '/layout',
    name: 'DashboardLayout',
    component: () => import('@/views/DashboardLayoutView.vue'),
  },
]

const router = createRouter({
  history: createWebHistory(basePath),
  routes,
})

// Block admin-only routes for non-admins. The store is populated before
// the very first navigation finishes (App.vue runs auth.init() at mount,
// and route loading is async), but to be safe we redirect to "/" if no
// user or wrong role.
router.beforeEach((to) => {
  if (to.meta.requiresAdmin) {
    const auth = useAuthStore()
    if (auth.role !== 'admin') {
      return { path: '/' }
    }
  }
  return true
})

export default router
