import { createRouter, createWebHistory } from 'vue-router'
import Home from '../views/Home.vue'
import Listen from '../views/Listen.vue'
import DJDashboard from '../views/DJDashboard.vue'
import Admin from '../views/Admin.vue'

const routes = [
  { path: '/', name: 'Home', component: Home },
  { path: '/radio/:stationId', name: 'Listen', component: Listen },
  { path: '/dj/:stationId', name: 'DJDashboard', component: DJDashboard },
  { path: '/admin', name: 'Admin', component: Admin },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
