export const USER_ROLES = [
  {
    value: 'Chat user',
    label: 'Chat user',
    connector: 'can access',
    main: 'Chats',
    bgColor: "background.buttonPrimaryOutlineHover",
    permissions: {
      sections: [
        {
          title: 'Access to Chats',
          items: [
            'Interact with Chats',
            'Add data sources and tools available in their catalogs to chats'
          ]
        }
      ]
    }
  },
  {
    value: 'Developer',
    label: 'Developer',
    connector: 'can access',
    main: 'AI portal and Chats',
    bgColor: "background.surfaceBrandDefaultPortal",
    permissions: {
      sections: [
        {
          title: 'Access to Chats',
          items: [
            'Interact with Chats',
            'Add data sources and tools available in their catalogs to chats'
          ]
        },
        {
          title: 'Access to AI Portal',
          items: [
            'Use Apps created by the admin',
            'Create and delete their own apps with LLM providers and data sources available in their catalogs'
          ]
        }
      ]
    }
  },
  {
    value: 'Admin',
    label: 'Admin',
    connector: 'can access',
    main: 'Admin, AI portal and Chats',
    bgColor: "background.surfaceBrandDefaultDashboard",
    permissions: {
      sections: [
        {
          title: 'Access to Chats',
          items: [
            'Interact with Chats',
            'Add data sources and tools available in their catalogs to chats'
          ]
        },
        {
          title: 'Access to AI Portal',
          items: [
            'Use Apps created by the admin',
            'Create and delete their own apps with LLM providers and data sources available in their catalogs'
          ]
        },
        {
          title: 'Access to Administration',
          items: [
            'CRUD LLM providers, data sources, tools, filters, middleware, Apps, Chats and catalogs',
            'Add, edit and delete Chat users and Developers.',
            'Add, edit and delete Teams.',
            'Monitor usage, iterations, and costs (set up budgets).'
          ]
        }
      ]
    }
  }
];

export const IsAdminRole = (role) => {
  return role === 'Admin' || role === 'Super Admin';
}