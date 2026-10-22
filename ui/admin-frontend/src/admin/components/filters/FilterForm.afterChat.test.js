import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "../../utils/testTheme";
// The chat Markdown views load first, as when a user opens chat and then
// the admin filter editor in the same page session. Their Prism build
// (refractor) patches the shared prismjs module and removes the global
// Prism. Before the fix, importing FilterForm after them threw "Prism is
// not defined", and Prism.highlight returned hast objects that crashed the
// script editor.
import MarkdownMessage from "../../../portal/components/chat/MarkdownMessage";
import { PrismHighlighter } from "../../../portal/components/chat-v2/MarkdownText";
import FilterForm from "./FilterForm";
import highlightScript from "./highlightScript";

jest.mock("prismjs/themes/prism-tomorrow.css", () => ({}));
jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn() },
}));
jest.mock("../common/EdgeAvailabilitySection", () => () => null);
jest.mock("./ScriptTestPanel", () => () => null);

const apiClient = require("../../utils/apiClient").default;

const SCRIPT = "const blocked = true; // keep";

const saved = {
  attributes: {
    name: "Script filter",
    description: "",
    script: btoa(SCRIPT), // stored base64
    kind: "script",
    response_filter: false,
    namespace: "",
  },
};

describe("FilterForm after the chat Markdown views", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockImplementation((url) => Promise.resolve(url === "/filters/7" ? { data: saved } : { data: [] }));
  });

  it("renders chat code blocks, then a working script editor", async () => {
    const chat = render(
      <>
        <MarkdownMessage content={"```js\nconst x = 1;\n```"} />
        <PrismHighlighter language="python" code={"def f():\n    return 1"} />
      </>,
    );
    // eslint-disable-next-line testing-library/no-node-access, testing-library/no-container
    expect(chat.container.querySelectorAll("code span").length).toBeGreaterThan(2);
    chat.unmount();

    expect(typeof highlightScript(SCRIPT)).toBe("string");

    const { container } = render(
      <ThemeProvider theme={testTheme}>
        <MemoryRouter initialEntries={["/admin/filters/edit/7"]}>
          <Routes>
            <Route path="/admin/filters/edit/:id" element={<FilterForm />} />
          </Routes>
        </MemoryRouter>
      </ThemeProvider>,
    );
    expect(await screen.findByDisplayValue(SCRIPT)).toBeInTheDocument();
    // eslint-disable-next-line testing-library/no-node-access, testing-library/no-container
    expect(container.querySelector("pre .token.keyword")).toHaveTextContent("const");
    // eslint-disable-next-line testing-library/no-node-access, testing-library/no-container
    expect(container.querySelector("pre .token.comment")).toHaveTextContent("// keep");
  });
});
