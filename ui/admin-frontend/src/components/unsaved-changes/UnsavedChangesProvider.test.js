import React, { useState } from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { createTheme } from "@mui/material";
import { MemoryRouter, Routes, Route, Link, useLocation } from "react-router-dom";
import {
  UnsavedChangesProvider,
  useUnsavedChangesGuard,
  useConfirmNavigation,
  useUnsavedChanges,
} from "./UnsavedChangesProvider";

jest.mock("../../components/common/Icon", () => {
  return function MockIcon(props) {
    return <div data-testid="mock-icon">{props.name}</div>;
  };
});

const theme = createTheme({
  palette: {
    text: { defaultSubdued: "rgba(0, 0, 0, 0.6)", criticalDefault: "#a00" },
    border: { neutralDefault: "#e0e0e0", criticalDefault: "#f00", criticalDefaultSubdue: "#fdd" },
    custom: { white: "#fff" },
    background: {
      buttonCritical: "#f00",
      buttonCriticalHover: "#c00",
      paper: "#fff",
      surfaceCriticalDefault: "#fee",
      surfaceNeutralHover: "#eee",
    },
  },
});

// A page that can be flipped dirty/clean, with the links the guard must catch.
const FormPage = () => {
  const [dirty, setDirty] = useState(false);
  useUnsavedChangesGuard(dirty);
  const confirmNavigation = useConfirmNavigation();
  const { isDirty } = useUnsavedChanges();
  return (
    <div>
      <h1>Form page</h1>
      <span data-testid="registry-dirty">{String(isDirty)}</span>
      <button type="button" onClick={() => setDirty(true)}>
        make dirty
      </button>
      <button type="button" onClick={() => setDirty(false)}>
        make clean
      </button>
      <Link to="/users">Users</Link>
      {/* jsdom cannot navigate; the bubble-phase preventDefault only silences
          that, the guard has already decided in the capture phase. */}
      <a href="https://example.com/elsewhere" onClick={(e) => e.preventDefault()}>
        External
      </a>
      <a href="/users" target="_blank" onClick={(e) => e.preventDefault()}>
        New tab
      </a>
      <button type="button" onClick={() => confirmNavigation(() => setDirty(false))}>
        programmatic
      </button>
    </div>
  );
};

const WhereAmI = () => {
  const location = useLocation();
  const { isDirty } = useUnsavedChanges();
  return (
    <>
      <span data-testid="pathname">{location.pathname}</span>
      {/* Lives outside the routed page, so it survives the form unmounting. */}
      {location.pathname !== "/form" && <span data-testid="registry-dirty">{String(isDirty)}</span>}
    </>
  );
};

const renderApp = () =>
  render(
    <ThemeProvider theme={theme}>
      <MemoryRouter initialEntries={["/form"]}>
        <UnsavedChangesProvider>
          <WhereAmI />
          <Routes>
            <Route path="/form" element={<FormPage />} />
            <Route path="/users" element={<h1>Users page</h1>} />
          </Routes>
        </UnsavedChangesProvider>
      </MemoryRouter>
    </ThemeProvider>
  );

const dialog = () => screen.queryByTestId("unsaved-changes-dialog");

describe("UnsavedChangesProvider", () => {
  beforeEach(() => {
    jest.restoreAllMocks();
  });

  it("lets internal links through when nothing is dirty", () => {
    renderApp();
    fireEvent.click(screen.getByText("Users"));
    expect(screen.getByText("Users page")).toBeInTheDocument();
    expect(dialog()).toBeNull();
  });

  it("intercepts an internal link click while dirty and stays put on Stay", async () => {
    renderApp();
    fireEvent.click(screen.getByText("make dirty"));
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true");

    fireEvent.click(screen.getByText("Users"));
    expect(await screen.findByTestId("unsaved-changes-dialog")).toBeInTheDocument();
    expect(screen.getByText("Discard unsaved changes?")).toBeInTheDocument();
    // Still on the form, with the link's navigation cancelled.
    expect(screen.getByTestId("pathname")).toHaveTextContent("/form");

    fireEvent.click(screen.getByRole("button", { name: "Stay" }));
    await waitFor(() => expect(dialog()).toBeNull());
    expect(screen.getByText("Form page")).toBeInTheDocument();
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true");
  });

  it("navigates to the link target on Leave and clears the registry", async () => {
    renderApp();
    fireEvent.click(screen.getByText("make dirty"));
    fireEvent.click(screen.getByText("Users"));
    await screen.findByTestId("unsaved-changes-dialog");

    fireEvent.click(screen.getByRole("button", { name: "Leave without saving" }));
    expect(await screen.findByText("Users page")).toBeInTheDocument();
    expect(screen.getByTestId("pathname")).toHaveTextContent("/users");
    await waitFor(() => expect(dialog()).toBeNull());
  });

  it("does not intercept external links, new-tab links or modifier clicks", () => {
    renderApp();
    fireEvent.click(screen.getByText("make dirty"));

    fireEvent.click(screen.getByText("External"));
    expect(dialog()).toBeNull();

    fireEvent.click(screen.getByText("New tab"));
    expect(dialog()).toBeNull();

    // A ctrl-click on the internal link is "open in new tab" and is left alone.
    fireEvent.click(screen.getByText("Users"), { ctrlKey: true });
    expect(dialog()).toBeNull();
    expect(screen.getByText("Form page")).toBeInTheDocument();
  });

  it("confirmNavigation runs the callback at once when clean and only after Leave when dirty", async () => {
    renderApp();
    // Clean: the callback (which sets dirty=false) runs immediately, no dialog.
    fireEvent.click(screen.getByText("programmatic"));
    expect(dialog()).toBeNull();

    fireEvent.click(screen.getByText("make dirty"));
    fireEvent.click(screen.getByText("programmatic"));
    expect(await screen.findByTestId("unsaved-changes-dialog")).toBeInTheDocument();
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("true");

    fireEvent.click(screen.getByRole("button", { name: "Leave without saving" }));
    await waitFor(() => expect(dialog()).toBeNull());
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false");
  });

  it("registers beforeunload only while dirty", () => {
    const addSpy = jest.spyOn(window, "addEventListener");
    const removeSpy = jest.spyOn(window, "removeEventListener");
    renderApp();
    const beforeUnloadAdds = () => addSpy.mock.calls.filter(([type]) => type === "beforeunload");
    const beforeUnloadRemoves = () => removeSpy.mock.calls.filter(([type]) => type === "beforeunload");

    expect(beforeUnloadAdds()).toHaveLength(0);

    fireEvent.click(screen.getByText("make dirty"));
    expect(beforeUnloadAdds()).toHaveLength(1);
    expect(beforeUnloadRemoves()).toHaveLength(0);

    // The handler asks the browser to prompt.
    const handler = beforeUnloadAdds()[0][1];
    const event = { preventDefault: jest.fn(), returnValue: undefined };
    handler(event);
    expect(event.preventDefault).toHaveBeenCalled();
    expect(event.returnValue).toBe("");

    fireEvent.click(screen.getByText("make clean"));
    expect(beforeUnloadRemoves()).toHaveLength(1);
  });

  it("steps history back to the form on popstate while dirty and replays it on Leave", async () => {
    const goSpy = jest.spyOn(window.history, "go").mockImplementation(() => {});
    renderApp();

    // The form sits at router index 3; the user then lands on index 2 (back).
    window.history.replaceState({ idx: 3 }, "", "/form");
    fireEvent.click(screen.getByText("make dirty"));

    const stopImmediate = jest.fn();
    act(() => {
      window.history.replaceState({ idx: 2 }, "", "/somewhere-else");
      const event = new Event("popstate");
      event.stopImmediatePropagation = stopImmediate;
      window.dispatchEvent(event);
    });

    // React-router never saw the event, and history was moved back by one.
    expect(stopImmediate).toHaveBeenCalled();
    expect(goSpy).toHaveBeenCalledWith(1);
    expect(await screen.findByTestId("unsaved-changes-dialog")).toBeInTheDocument();
    expect(screen.getByText("Form page")).toBeInTheDocument();

    // The restoring popstate (from go(1)) is swallowed without a prompt.
    goSpy.mockClear();
    act(() => {
      window.history.replaceState({ idx: 3 }, "", "/form");
      window.dispatchEvent(new Event("popstate"));
    });
    expect(goSpy).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Leave without saving" }));
    expect(goSpy).toHaveBeenCalledWith(-1);
    await waitFor(() => expect(dialog()).toBeNull());
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false");
  });

  it("ignores popstate when nothing is dirty", () => {
    const goSpy = jest.spyOn(window.history, "go").mockImplementation(() => {});
    renderApp();
    const stopImmediate = jest.fn();
    act(() => {
      const event = new Event("popstate");
      event.stopImmediatePropagation = stopImmediate;
      window.dispatchEvent(event);
    });
    expect(stopImmediate).not.toHaveBeenCalled();
    expect(goSpy).not.toHaveBeenCalled();
    expect(dialog()).toBeNull();
  });

  it("unregisters a form when it unmounts", async () => {
    renderApp();
    fireEvent.click(screen.getByText("make dirty"));
    fireEvent.click(screen.getByText("Users"));
    await screen.findByTestId("unsaved-changes-dialog");
    fireEvent.click(screen.getByRole("button", { name: "Leave without saving" }));
    await screen.findByText("Users page");
    await waitFor(() => expect(dialog()).toBeNull());
    // The form page's guard unregistered on unmount, so nothing is left
    // behind: the registry reads clean from the Users page.
    expect(screen.getByTestId("registry-dirty")).toHaveTextContent("false");
  });
});

describe("hooks without a provider", () => {
  const Bare = () => {
    useUnsavedChangesGuard(true);
    const confirmNavigation = useConfirmNavigation();
    const [ran, setRan] = useState(false);
    return (
      <button type="button" onClick={() => confirmNavigation(() => setRan(true))}>
        {ran ? "ran" : "go"}
      </button>
    );
  };

  it("confirmNavigation runs its callback immediately", () => {
    render(<Bare />);
    fireEvent.click(screen.getByText("go"));
    expect(screen.getByText("ran")).toBeInTheDocument();
  });
});
