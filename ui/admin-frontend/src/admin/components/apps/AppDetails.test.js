import React from "react";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import AppDetails from "./AppDetails";
import { renderWithRouterAndTheme } from "../../../test-utils/render-with-theme";
import apiClient from "../../utils/apiClient";
import agentService from "../../services/agentService";

const mockNavigate = jest.fn();

jest.mock("react-chartjs-2", () => ({
  Line: () => <div data-testid="line-chart" />,
}));

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: {
    get: jest.fn(),
    patch: jest.fn(),
  },
  appToolAPI: {
    getAppTools: jest.fn(),
  },
}));

jest.mock("../../services/agentService", () => ({
  __esModule: true,
  default: {
    listAgents: jest.fn(),
  },
}));

jest.mock("../../context/EditionContext", () => ({
  useEdition: () => ({ isEnterprise: false }),
}));

jest.mock("react-router-dom", () => {
  const actual = jest.requireActual("react-router-dom");
  return {
    ...actual,
    useNavigate: () => mockNavigate,
    useParams: () => ({ id: "42" }),
  };
});

const mockApp = {
  id: "42",
  attributes: {
    name: "Support App",
    description: "Routes support requests",
    user_id: "7",
    is_orphaned: false,
    llms: [{ id: "llm-1", attributes: { name: "GPT Support" } }],
    datasources: [{ id: "ds-1", attributes: { name: "Knowledge Base" } }],
    tools: [{ id: "tool-1", attributes: { name: "Ticket Lookup" } }],
    plugin_resources: [],
    monthly_budget: null,
  },
};

const mockUser = {
  id: "7",
  attributes: {
    name: "Alex Admin",
  },
};

const analyticsResponse = {
  data: {
    labels: [],
    datasets: [],
  },
};

describe("AppDetails", () => {
  beforeEach(() => {
    jest.clearAllMocks();

    agentService.listAgents.mockResolvedValue({ data: [] });

    apiClient.get.mockImplementation((url) => {
      if (url === "/apps/42") {
        return Promise.resolve({ data: { data: mockApp } });
      }

      if (url === "/users/7") {
        return Promise.resolve({ data: { data: mockUser } });
      }

      if (url === "/analytics/proxy-logs-for-app") {
        return Promise.resolve({
          data: {
            data: [],
            meta: {
              total_count: 0,
              total_pages: 0,
            },
          },
        });
      }

      if (
        url === "/analytics/usage" ||
        url === "/analytics/budget-usage-for-app" ||
        url === "/analytics/app-interactions-over-time"
      ) {
        return Promise.resolve(analyticsResponse);
      }

      return Promise.reject(new Error(`Unhandled GET ${url}`));
    });
  });

  test("renders a single edit action before proxy logs", async () => {
    renderWithRouterAndTheme(<AppDetails />);

    const editButton = await screen.findByRole("button", { name: /edit app/i });
    const proxyLogsHeading = await screen.findByRole("heading", { name: /proxy logs/i });

    expect(screen.getAllByRole("button", { name: /edit app/i })).toHaveLength(1);
    expect(
      editButton.compareDocumentPosition(proxyLogsHeading) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy();
  });

  test("navigates to the app edit page from the header action", async () => {
    renderWithRouterAndTheme(<AppDetails />);

    const editButton = await screen.findByRole("button", { name: /edit app/i });
    fireEvent.click(editButton);

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith("/admin/apps/edit/42");
    });
  });
});
