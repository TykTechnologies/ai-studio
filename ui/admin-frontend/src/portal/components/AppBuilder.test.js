import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../admin/utils/testTheme";
import AppBuilder from "./AppBuilder";
import {
  UnsavedChangesProvider,
  useUnsavedChanges,
} from "../../components/unsaved-changes";

jest.mock("../../admin/utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn() },
}));
jest.mock("../../components/common/Icon", () => {
  return function MockIcon(props) {
    return <div data-testid="mock-icon">{props.name}</div>;
  };
});
jest.mock("../../admin/components/common/relationship-picker", () =>
  require("../../test-utils/component-mocks").relationshipPickerMock,
);

const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const pubClient = require("../../admin/utils/pubClient").default;

const llms = [
  { id: "1", attributes: { name: "OpenAI" } },
  { id: "2", attributes: { name: "Anthropic" } },
];
const dataSources = [{ id: "3", attributes: { name: "Docs" } }];
const tools = [{ id: "4", attributes: { name: "Weather" } }];
const resourceTypes = [
  { plugin_id: 3, slug: "vector-db", name: "Vector DBs", instances: [{ id: "i1", name: "Pinecone" }] },
];

const DirtyProbe = () => {
  const { isDirty } = useUnsavedChanges();
  return <span data-testid="registry-dirty">{String(isDirty)}</span>;
};

const renderBuilder = ({ path = "/portal/apps/new" } = {}) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[path]}>
        <UnsavedChangesProvider>
          <AppBuilder />
          <DirtyProbe />
        </UnsavedChangesProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );

const picker = (itemLabel) =>
  screen.getAllByTestId("relationship-picker").find((el) => el.dataset.itemLabel === itemLabel);

const pickerItems = (itemLabel) =>
  within(picker(itemLabel)).queryAllByTestId("relationship-picker-item").map((el) => el.textContent);

describe("AppBuilder pickers, name default and commit semantics", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pubClient.get.mockImplementation((url) => {
      if (url === "/common/accessible-datasources") return Promise.resolve({ data: dataSources });
      if (url === "/common/accessible-llms") return Promise.resolve({ data: llms });
      if (url === "/common/accessible-tools") return Promise.resolve({ data: tools });
      if (url === "/common/accessible-plugin-resources") return Promise.resolve({ data: { data: resourceTypes } });
      return Promise.resolve({ data: [] });
    });
    pubClient.post.mockResolvedValue({ data: { data: { id: "77" } } });
  });

  // The "pick, then Add" blocks with chips were one of five idioms for the
  // same relationship (F-05); "My New App" was submitted verbatim as a name.
  it("starts with an empty required name and one compact picker per resource", async () => {
    renderBuilder();
    await waitFor(() => expect(picker("LLM provider")).toBeTruthy());

    expect(screen.getByRole("textbox", { name: /App Name/ })).toHaveValue("");
    expect(screen.getByRole("textbox", { name: /App Name/ })).toBeRequired();
    for (const label of ["data source", "LLM provider", "tool", "vector dbs"]) {
      expect(picker(label)).toHaveAttribute("data-variant", "compact");
    }
    expect(within(picker("LLM provider")).getByTestId("relationship-picker-options-count")).toHaveTextContent("2");
    // The old "Select LLM" dropdown + "Add" button pairs are gone.
    expect(screen.queryByRole("combobox", { name: /Select LLM/ })).not.toBeInTheDocument();
    expect(screen.queryByText("Select Data Source")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create App" })).toBeDisabled();
  });

  it("shows a ?llm= preselection as a member without dirtying the form, and saves what the pickers hold", async () => {
    renderBuilder({ path: "/portal/apps/new?llm=2" });
    await waitFor(() => expect(pickerItems("LLM provider")).toEqual(["Anthropic"]));
    await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false"));

    fireEvent.change(screen.getByRole("textbox", { name: /App Name/ }), { target: { value: "Support bot" } });
    fireEvent.change(screen.getByRole("textbox", { name: /Description/ }), { target: { value: "Helps" } });
    await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true"));
    fireEvent.click(within(picker("LLM provider")).getByTestId("relationship-picker-add"));
    fireEvent.click(within(picker("vector dbs")).getByTestId("relationship-picker-add"));
    expect(pickerItems("LLM provider")).toEqual(["Anthropic", "OpenAI"]);

    fireEvent.click(screen.getByRole("button", { name: "Create App" }));

    await waitFor(() =>
      expect(pubClient.post).toHaveBeenCalledWith("/common/apps", {
        name: "Support bot",
        description: "Helps",
        data_source_ids: [],
        llm_ids: [2, 1],
        tool_ids: [],
        plugin_resources: [{ plugin_id: 3, resource_type_slug: "vector-db", instance_ids: ["i1"] }],
      }),
    );
    expect(await screen.findByText("App Submitted")).toBeInTheDocument();
    // markSaved() ran before the success screen replaced the form.
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false");
  });

  it("has a Cancel that returns to the apps list when clean and prompts when dirty", async () => {
    renderBuilder();
    await waitFor(() => expect(picker("LLM provider")).toBeTruthy());
    await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false"));

    const cancel = screen.getByRole("button", { name: "Cancel" });
    fireEvent.click(cancel);
    expect(mockNavigate).toHaveBeenCalledWith("/portal/apps");

    fireEvent.click(within(picker("tool")).getByTestId("relationship-picker-add"));
    await waitFor(() => expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true"));

    mockNavigate.mockClear();
    fireEvent.click(cancel);
    expect(mockNavigate).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });
});
