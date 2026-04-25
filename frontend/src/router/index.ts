import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'

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
    path: '/design-preview',
    name: 'DesignPreview',
    component: () => import('@/views/DesignPreview.vue'),
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
