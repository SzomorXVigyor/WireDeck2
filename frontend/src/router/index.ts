import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useSiteStore } from '../stores/sites'

// Eagerly loaded: Auth views & Service/Member Surface
import LoginView from '../views/auth/LoginView.vue'
import AcceptInviteView from '../views/auth/AcceptInviteView.vue'
import ForgotPasswordView from '../views/auth/ForgotPasswordView.vue'
import ResetPasswordView from '../views/auth/ResetPasswordView.vue'
import PasswordChangedView from '../views/auth/PasswordChangedView.vue'
import OperatorLayout from '../layouts/OperatorLayout.vue'
import HomeRedirectView from '../modules/operator/HomeRedirectView.vue'
import SiteOverviewView from '../modules/operator/SiteOverviewView.vue'
import ProfileView from '../modules/operator/ProfileView.vue'
import PreferencesView from '../modules/operator/PreferencesView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    // Public auth routes
    { path: '/login', name: 'login', component: LoginView, meta: { public: true } },
    { path: '/accept-invite', name: 'accept-invite', component: AcceptInviteView, meta: { public: true } },
    { path: '/forgot-password', name: 'forgot-password', component: ForgotPasswordView, meta: { public: true } },
    { path: '/reset-password', name: 'reset-password', component: ResetPasswordView, meta: { public: true } },
    { path: '/password-changed', name: 'password-changed', component: PasswordChangedView, meta: { public: true } },

    // Surface 1: Site Services Shell (Site Member, Site Operator, Platform Admin)
    {
      path: '/',
      component: OperatorLayout,
      meta: { requiresAuth: true },
      children: [
        { path: '', name: 'home', component: HomeRedirectView },
        { path: 'profile', name: 'profile', component: ProfileView },
        { path: 'preferences', name: 'preferences', component: PreferencesView },
        { path: 'profile/preferences', redirect: '/preferences' },
        {
          path: 'alarms',
          name: 'global-alarms',
          component: () => import('../modules/operator/AlarmCenterView.vue'),
        },
        { path: 'sites/:siteId/overview', name: 'site-overview', component: SiteOverviewView },
        {
          path: 'sites/:siteId/webview',
          name: 'site-webview',
          component: () => import('../modules/operator/WebView.vue'),
          meta: { requireFlag: 'webviewEnabled' },
        },
        {
          path: 'sites/:siteId/webview/:dashboardId',
          name: 'site-webview-dashboard',
          component: () => import('../modules/operator/WebView.vue'),
          meta: { requireFlag: 'webviewEnabled' },
        },
        {
          path: 'sites/:siteId/vnc',
          name: 'site-vnc',
          component: () => import('../modules/operator/VncView.vue'),
          meta: { requireFlag: 'vncEnabled' },
        },
        {
          path: 'sites/:siteId/history',
          name: 'site-history',
          component: () => import('../modules/operator/HistoryView.vue'),
          meta: { requireFlag: 'webviewEnabled' },
        },
        {
          path: 'sites/:siteId/alarms',
          name: 'site-alarms',
          component: () => import('../modules/operator/AlarmCenterView.vue'),
          meta: { requireFlag: 'webviewEnabled' },
        },
      ],
    },

    // Standalone full-viewport VNC console tab (opened in a new tab from VNC cards)
    {
      path: '/sites/:siteId/vnc/:targetId',
      name: 'site-vnc-session',
      component: () => import('../modules/operator/VncSessionView.vue'),
      meta: { requiresAuth: true, requireFlag: 'vncEnabled' },
    },

    // Surface 2: Site Operator Workspace (lazy-loaded chunk - Members, VNC edge points, WebView pages)
    // Never exposes WireGuard config or site-level feature flags.
    {
      path: '/sites/:siteId/manage',
      component: () => import('../layouts/SiteAdminLayout.vue'),
      meta: { requiresAuth: true, requiresSiteOperator: true },
      children: [
        { path: '', redirect: (to) => `/sites/${to.params.siteId}/manage/tunnel` },
        {
          path: 'tunnel',
          name: 'site-operator-tunnel',
          component: () => import('../modules/site-admin/SiteTunnelView.vue'),
        },
        {
          path: 'members',
          name: 'site-operator-members',
          component: () => import('../modules/site-admin/SiteSettingsView.vue'),
        },
        {
          path: 'vnc-targets',
          name: 'site-operator-vnc',
          component: () => import('../modules/site-admin/SiteVncTargetsView.vue'),
        },
        {
          path: 'webview',
          name: 'site-operator-webview',
          component: () => import('../modules/site-admin/SiteWebViewManageView.vue'),
        },
      ],
    },

    // Surface 3: Platform Admin Console (lazy-loaded chunk - Sites, Features, WireGuard, Users, Runners)
    {
      path: '/admin',
      component: () => import('../layouts/PlatformAdminLayout.vue'),
      meta: { requiresAuth: true, requiresPlatformAdmin: true },
      children: [
        { path: '', redirect: '/admin/sites' },
        {
          path: 'sites',
          name: 'admin-sites',
          component: () => import('../modules/platform-admin/AdminSitesView.vue'),
        },
        {
          path: 'sites/:siteId',
          name: 'admin-site-detail',
          component: () => import('../modules/platform-admin/AdminSiteDetailView.vue'),
        },
        {
          path: 'users',
          name: 'admin-users',
          component: () => import('../modules/platform-admin/AdminUsersView.vue'),
        },
        {
          path: 'runners',
          name: 'admin-runners',
          component: () => import('../modules/platform-admin/AdminRunnersView.vue'),
        },
        {
          path: 'runners/:siteId',
          name: 'admin-runner-detail',
          component: () => import('../modules/platform-admin/AdminRunnerDetailView.vue'),
        },
        {
          path: 'profile',
          name: 'admin-profile',
          component: ProfileView,
        },
        {
          path: 'preferences',
          name: 'admin-preferences',
          component: PreferencesView,
        },
      ],
    },

    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (!auth.initialized) {
    await auth.init()
  }

  if (to.meta.public) {
    if (auth.isAuthenticated && to.name === 'login') {
      return { name: 'home' }
    }
    return true
  }

  if (!auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }

  if (to.meta.requiresPlatformAdmin && !auth.isPlatformAdmin) {
    return { name: 'home' }
  }

  const siteId = typeof to.params.siteId === 'string' ? to.params.siteId : undefined
  if (siteId) {
    const sites = useSiteStore()
    if (sites.sites.length === 0) {
      await sites.fetchSites()
    }
    const site = sites.byId(siteId)
    if (!site && !auth.isPlatformAdmin) {
      return { name: 'home' }
    }
    if (to.meta.requiresSiteOperator && !sites.canOperateSite(siteId)) {
      return { name: 'site-overview', params: { siteId } }
    }
    const flag = to.meta.requireFlag as 'vncEnabled' | 'webviewEnabled' | undefined
    if (flag && site && !site[flag]) {
      return { name: 'site-overview', params: { siteId } }
    }
  }

  return true
})

export default router
