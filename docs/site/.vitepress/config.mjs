import { defineConfig } from 'vitepress'

// https://vitepress.dev/reference/site-config
export default defineConfig({
  // Set base path conditionally
  base: process.env.NODE_ENV === 'production' ? '/ai-studio/' : '/',
  // Output to dist folder for Go embed (avoids .vitepress dot-prefix issue)
  outDir: 'dist',
  title: "Tyk AI Studio",
  description: "Tyk AI Studio - Accelerate AI innovation without sacrificing control",
  ignoreDeadLinks: true,
  appearance: 'force-dark', // Force dark mode
  themeConfig: {
    // https://vitepress.dev/reference/default-theme-config
    logo: '/logo.png',
    nav: [
      { text: 'Quickstart', link: '/docs/quickstart' }
    ],

    sidebar: [
      {
        text: 'Introduction',
        items: [
          { text: 'Overview', link: '/' }, // Assuming index.md is the overview
          { text: 'Quickstart', link: '/docs/quickstart' },
          { text: 'Core Concepts', link: '/docs/core-concepts' } // New
        ]
      },
      {
        text: 'Getting Started',
        items: [
          { text: 'Installation (Helm/K8s)', link: '/docs/deployment-helm-k8s' }, // Renamed
          { text: 'Initial Configuration', link: '/docs/configuration' } // Renamed
        ]
      },
      {
        text: 'Core Features',
        items: [
          { text: 'AI Gateway', link: '/docs/proxy' }, // Renamed from Proxy & API Gateway
          { text: 'AI Portal', link: '/docs/ai-portal' }, // New
          { text: 'Chat Interface', link: '/docs/chat-interface' }, // Moved up
          { text: 'LLM Management', link: '/docs/llm-management' },
          { text: 'Model Router (Enterprise)', link: '/docs/model-router' }, // Enterprise model routing
          { text: 'Tools & Extensibility', link: '/docs/tools' },
          { text: 'Data Sources & RAG', link: '/docs/datasources-rag' },
          { text: 'Filters & Policies', link: '/docs/filters' }
        ]
      },
      {
        text: 'Administration',
        items: [
          { text: 'User Management & RBAC', link: '/docs/user-management' }, // New (merge users, groups)
          { text: 'SSO Integration', link: '/docs/sso' }, // New
          { text: 'Secrets Management', link: '/docs/secrets' }, // Keep secrets
          { text: 'Budget Control', link: '/docs/budgeting' }, // New
          { text: 'Analytics & Monitoring', link: '/docs/analytics' }, // New (replace dashboard)
          { text: 'Notifications', link: '/docs/notifications' }, // New
          { text: 'Edge Gateways (Enterprise)', link: '/docs/edge-gateways' } // Enterprise hub-spoke management
          // Removed: apps, model-prices, call-settings (to be merged)
        ]
      },
      {
        text: 'Plugins & Extensions',
        items: [
          { text: 'Overview', link: '/docs/plugins-overview' },
          { text: 'Microgateway Plugins', link: '/docs/plugins-microgateway' },
          { text: 'AI Studio UI Plugins', link: '/docs/plugins-studio-ui' },
          { text: 'AI Studio Agent Plugins', link: '/docs/plugins-studio-agent' },
          { text: 'Manifests & Permissions', link: '/docs/plugins-manifests' },
          { text: 'Deployment Options', link: '/docs/plugins-deployment' },
          { text: 'SDK Reference', link: '/docs/plugins-sdk' },
          { text: 'Service API Reference', link: '/docs/plugins-service-api' }
        ]
      }
    ],

    search: {
      provider: 'local'
    },

    footer: {
      message: 'Accelerating AI innovation with Tyk AI Studio.',
      copyright: 'Copyright 2025 Tyk Technologies'
    }
  },
  vite: {
    server: {
      fs: {
        // Allow serving files from one level up to include workspace root node_modules
        allow: ['/Users/leonidbugaev/go/src/']
      }
    }
  },
  srcExclude: ['**/themes/**']
})
