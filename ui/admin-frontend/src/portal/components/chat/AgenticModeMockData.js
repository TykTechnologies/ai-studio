// Mock data and functions for agentic mode simulation
const mockZendeskResults = `{
  "total_results": 2,
  "tickets": [
    {
      "id": "ZD-4872",
      "subject": "ACME PoC - Initial Setup Issues",
      "status": "Solved",
      "priority": "High",
      "created_at": "2025-01-15T09:23:45Z",
      "updated_at": "2025-01-22T14:30:12Z",
      "description": "Customer reporting issues with initial setup of the ACME PoC environment. Authentication failures when connecting to the API.",
      "tags": ["acme", "poc", "api", "authentication"],
      "assignee": "Sarah Johnson",
      "requester": "John Miller (ACME)",
      "jira_references": ["PROJ-423", "AUTH-187"]
    },
    {
      "id": "ZD-4901",
      "subject": "ACME PoC - Data Integration Questions",
      "status": "Open",
      "priority": "Medium",
      "created_at": "2025-01-28T11:45:22Z",
      "updated_at": "2025-02-24T16:12:33Z",
      "description": "ACME team has questions about integrating their existing data warehouse with our solution. Need technical specifications and best practices.",
      "tags": ["acme", "poc", "data-integration", "warehouse"],
      "assignee": "Michael Chen",
      "requester": "Lisa Wong (ACME)",
      "jira_references": ["INTG-512", "DATA-309"]
    }
  ]
}`;

const mockJiraResults = `{
  "tickets": [
    {
      "key": "PROJ-423",
      "summary": "Set up ACME PoC environment with enhanced security",
      "status": "Done",
      "assignee": "Alex Rivera",
      "created": "2025-01-14T08:12:33Z",
      "updated": "2025-01-21T16:45:22Z",
      "priority": "High",
      "description": "Create isolated environment for ACME PoC with all security measures implemented according to their compliance requirements.",
      "resolution": "Environment created with enhanced security protocols. Documentation shared with ACME team."
    },
    {
      "key": "AUTH-187",
      "summary": "Fix authentication issues in ACME PoC API integration",
      "status": "Done",
      "assignee": "Priya Patel",
      "created": "2025-01-16T10:23:45Z",
      "updated": "2025-01-20T14:30:12Z",
      "priority": "Critical",
      "description": "ACME users experiencing authentication failures when connecting to API. Investigate and resolve.",
      "resolution": "Issue traced to misconfigured OAuth settings. Fixed and verified working with ACME team."
    },
    {
      "key": "INTG-512",
      "summary": "Create data integration plan for ACME data warehouse",
      "status": "In Progress",
      "assignee": "Carlos Mendez",
      "created": "2025-01-29T09:15:33Z",
      "updated": "2025-02-23T11:42:18Z",
      "priority": "Medium",
      "description": "Develop comprehensive plan for integrating ACME's existing data warehouse with our platform. Include data mapping, transformation rules, and validation procedures.",
      "comments": [
        {
          "author": "Carlos Mendez",
          "date": "2025-02-20T14:22:33Z",
          "content": "Initial data mapping complete. Working on transformation rules. Meeting with ACME data team scheduled for Feb 27."
        }
      ]
    },
    {
      "key": "DATA-309",
      "summary": "Implement custom data connectors for ACME legacy systems",
      "status": "To Do",
      "assignee": "Sophia Kim",
      "created": "2025-02-02T13:45:22Z",
      "updated": "2025-02-24T15:30:42Z",
      "priority": "Medium",
      "description": "Develop custom connectors for ACME's legacy data systems that don't support standard protocols. Focus on reliability and error handling.",
      "dependencies": ["INTG-512"]
    },
    {
      "key": "PERF-218",
      "summary": "Optimize query performance for ACME data volume",
      "status": "In Progress",
      "assignee": "Marcus Johnson",
      "created": "2025-02-11T08:30:15Z",
      "updated": "2025-02-25T10:12:33Z",
      "priority": "High",
      "description": "Implement performance optimizations to handle ACME's high data volume. Focus on query optimization and caching strategies.",
      "comments": [
        {
          "author": "Marcus Johnson",
          "date": "2025-02-18T16:45:22Z",
          "content": "Initial benchmarks complete. Identified bottlenecks in query processing. Working on optimization plan."
        },
        {
          "author": "Technical Lead",
          "date": "2025-02-22T09:30:45Z",
          "content": "Consider implementing partitioning strategy for ACME's historical data to improve query performance."
        }
      ]
    },
    {
      "key": "OPT-143",
      "summary": "Implement caching layer for ACME PoC",
      "status": "To Do",
      "assignee": "Aisha Patel",
      "created": "2025-02-15T11:22:33Z",
      "updated": "2025-02-24T14:15:42Z",
      "priority": "Medium",
      "description": "Design and implement caching strategy to improve performance for ACME PoC. Consider Redis for distributed caching.",
      "dependencies": ["PERF-218"]
    },
    {
      "key": "PM-621",
      "summary": "Revise ACME PoC timeline and deliverables",
      "status": "In Progress",
      "assignee": "Jennifer Adams",
      "created": "2025-02-22T14:30:12Z",
      "updated": "2025-02-26T09:45:33Z",
      "priority": "Medium",
      "description": "Update project plan to accommodate ACME's request for 2-week extension. Adjust milestones and deliverables accordingly.",
      "comments": [
        {
          "author": "Jennifer Adams",
          "date": "2025-02-23T10:15:22Z",
          "content": "Draft revised timeline created. Reviewing impact on resource allocation and dependencies."
        },
        {
          "author": "Project Director",
          "date": "2025-02-25T15:30:42Z",
          "content": "Approved timeline extension. Please coordinate with all teams to ensure smooth transition to new schedule."
        }
      ]
    }
  ]
}`;

const mockFinalSummary = `Based on my analysis of the Zendesk tickets and JIRA issues, here's the current state of the ACME PoC:

## Current Status

1. **Initial Setup (Complete)**
   - Environment has been set up with enhanced security measures (PROJ-423)
   - Authentication issues have been resolved (AUTH-187)

2. **Data Integration (In Progress)**
   - Data integration planning is underway (INTG-512)
   - Initial data mapping is complete
   - Custom connectors for legacy systems are planned but not started (DATA-309)

3. **Performance Optimization (In Progress)**
   - Performance benchmarking has identified issues below expectations (ZD-5012)
   - Query optimization work has started (PERF-218)
   - Caching layer implementation is planned but not started (OPT-143)

4. **Timeline Management (In Progress)**
   - ACME has requested a 2-week extension due to internal resource constraints (ZD-5104)
   - Project timeline revision is in progress (PM-621)
   - Extension has been approved by Project Director

## Planned Actions

1. **Data Integration**
   - Meeting with ACME data team scheduled for February 27th
   - Complete transformation rules for data warehouse integration
   - Begin implementation of custom connectors for legacy systems

2. **Performance Optimization**
   - Implement query optimization recommendations
   - Consider data partitioning strategy for historical data
   - Implement Redis-based caching layer

3. **Project Management**
   - Finalize revised timeline with 2-week extension
   - Coordinate with all teams on new schedule
   - Adjust resource allocation as needed

## Key Dates
- Data team meeting: February 27, 2025
- Revised PoC completion date: Approximately mid-March 2025 (pending final timeline approval)

Would you like me to provide more details on any specific aspect of the ACME PoC?`;

// Sequence of messages for the agentic mode simulation
const agenticModeSequence = [
  {
    content: "I'll help you find information about the ACME PoC, related Zendesk tickets, and JIRA tickets. Let me start gathering this information...",
    type: 'ai',
    delay: 1000
  },
  {
    content: ":::system\nPlanning approach to answer query about ACME PoC status\nI'll use the zendesk_search tool to find relevant tickets about the ACME PoC\n:::",
    type: 'system',
    delay: 1500
  },
  {
    content: ":::systemfunctionCall: zendesk_search::::::system functionCall: zendesk_search:::",
    type: 'system',
    delay: 1000
  },
  {
    content: ":::systemfunctionParams: {\n  \"query\": \"ACME PoC\",\n  \"time_range\": \"last_3_months\"\n}:::",
    type: 'system',
    delay: 1000
  },
  {
    content: ":::systemfunctionResults: " + mockZendeskResults + ":::",
    type: 'system',
    delay: 2000
  },
  {
    content: `I've found 2 Zendesk tickets related to the ACME PoC from the last 3 months. I'll analyze each ticket one by one to understand the current state.`,
    type: 'ai',
    delay: 1500
  },
  {
    content: ":::system\nStarting analysis of ticket ZD-4872 (ACME PoC - Initial Setup Issues)\n:::",
    type: 'system',
    delay: 1000
  },
  {
    content: ":::system\nfunctionCall: jira_ticket_lookup\n:::",
    type: 'system',
    delay: 1000
  },
  {
    content: ":::system\nfunctionParams: {\n  \"ticket_ids\": [\"PROJ-423\", \"AUTH-187\"]\n}\n:::",
    type: 'system',
    delay: 1000
  },
  {
    content: ":::system\nfunctionResults: " + mockJiraResults + "\n:::",
    type: 'system',
    delay: 2000
  },
  {
    content: `I've analyzed the first ticket (ZD-4872) about initial setup issues. This ticket is marked as Solved. The associated JIRA tickets (PROJ-423 and AUTH-187) show that the environment was set up with enhanced security measures and the authentication issues were resolved.`,
    type: 'ai',
    delay: 1500
  },
  {
    content: ":::system\nWork is not complete. Moving to analyze ticket ZD-4901 (ACME PoC - Data Integration Questions)\n:::",
    type: 'system',
    delay: 1000
  },
  {
    content: ":::system\nfunctionCall: jira_ticket_lookup\n:::",
    type: 'system',
    delay: 1000
  },
  {
    content: ":::system\nfunctionParams: {\n  \"ticket_ids\": [\"INTG-512\", \"DATA-309\"]\n}\n:::",
    type: 'system',
    delay: 1000
  },
  {
    content: ":::system\nfunctionResults: " + mockJiraResults + "\n:::",
    type: 'system',
    delay: 2000
  },
  {
    content: `I've analyzed the second ticket (ZD-4901) about data integration questions. This ticket is still Open. The associated JIRA tickets show that data integration planning is underway (INTG-512) with initial data mapping complete, and custom connectors for legacy systems are planned but not started (DATA-309).`,
    type: 'ai',
    delay: 1500
  },
  {
    content: ":::system\nAll tickets analyzed. Now preparing comprehensive summary of findings\n:::",
    type: 'system',
    delay: 1000
  },
  {
    content: ":::system\nfunctionCall: generate_summary\n:::",
    type: 'system',
    delay: 1000
  },
  {
    content: ":::system\nfunctionParams: {\n  \"topic\": \"ACME PoC status based on analyzed tickets\",\n  \"include_sections\": [\"current_status\", \"planned_actions\", \"key_dates\"],\n  \"format\": \"markdown\"\n}\n:::",
    type: 'system',
    delay: 1000
  },
  {
    content: ":::system\nfunctionResults: " + JSON.stringify({ summary: mockFinalSummary }) + "\n:::",
    type: 'system',
    delay: 2000
  },
  {
    content: mockFinalSummary,
    type: 'ai',
    delay: 1500
  }
];

// Function to simulate the agentic mode with proper agent loop and feedback
const simulateAgenticMode = (setMessages) => {
  let messageIndex = 0;

  const processNextMessage = () => {
    if (messageIndex >= agenticModeSequence.length) {
      return;
    }

    const message = agenticModeSequence[messageIndex];
    const newId = `mock_${message.type}_${Date.now()}_${messageIndex}`;

    setMessages(prev => [
      ...prev,
      {
        id: newId,
        type: message.type,
        content: message.content,
        isComplete: true
      }
    ]);

    messageIndex++;

    if (messageIndex < agenticModeSequence.length) {
      setTimeout(processNextMessage, message.delay);
    }
  };

  if (agenticModeSequence.length > 0) {
    processNextMessage();
  }
};

export default simulateAgenticMode;
