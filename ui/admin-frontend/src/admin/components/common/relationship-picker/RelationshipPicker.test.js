import React, { useState } from "react";
import { screen, fireEvent, waitFor, within, act } from "@testing-library/react";
import "@testing-library/jest-dom";
import { renderWithTheme } from "../../../../test-utils/render-with-theme";
import RelationshipPicker, { COMMIT_CAPTION } from "./RelationshipPicker";

const OPTIONS = [
  { id: "1", name: "OpenAI", email: "openai@example.com" },
  { id: "2", name: "Anthropic", email: "anthropic@example.com" },
  { id: "3", name: "Mistral", email: "mistral@example.com" },
];

// Controlled harness: the picker never owns its selection, so tests that add
// and then remove need a parent that feeds `onChange` back into `value`.
const Harness = ({ initial = [], onChange, ...props }) => {
  const [value, setValue] = useState(initial);
  return (
    <RelationshipPicker
      label="LLMs in this catalog"
      itemLabel="LLM"
      options={OPTIONS}
      value={value}
      onChange={(next) => {
        setValue(next);
        onChange?.(next);
      }}
      {...props}
    />
  );
};

// MUI resets an unfocused Autocomplete's input on every render, so focus it
// first, as a real user's click would.
const openAutocomplete = () => {
  const input = screen.getByRole("combobox", { name: "Add LLM" });
  fireEvent.focus(input);
  fireEvent.mouseDown(input);
  return input;
};

describe("RelationshipPicker (compact)", () => {
  it("renders the root, the heading, the empty text and the commit caption", () => {
    renderWithTheme(<Harness />);

    const root = screen.getByTestId("relationship-picker");
    expect(root).toHaveAttribute("aria-label", "LLMs in this catalog");
    expect(root).toHaveAttribute("data-variant", "compact");
    expect(screen.getByText("LLMs in this catalog")).toBeInTheDocument();
    expect(screen.getByTestId("relationship-picker-empty")).toHaveTextContent("No LLMs selected");
    expect(screen.getByTestId("relationship-picker-caption")).toHaveTextContent(COMMIT_CAPTION);
    expect(screen.getByTestId("relationship-picker-caption")).toHaveTextContent(
      "Changes apply when you save this form."
    );
  });

  it("honours a custom emptyText", () => {
    renderWithTheme(<Harness emptyText="Nothing here yet" />);
    expect(screen.getByTestId("relationship-picker-empty")).toHaveTextContent("Nothing here yet");
  });

  it("renders selected items as chips carrying data-item-id and a Remove aria-label", () => {
    renderWithTheme(<Harness initial={[OPTIONS[0]]} />);

    const chip = screen.getByText("OpenAI").closest(".MuiChip-root");
    expect(chip).toHaveAttribute("data-item-id", "1");
    expect(screen.getByLabelText("Remove OpenAI")).toBeInTheDocument();
    expect(screen.queryByTestId("relationship-picker-empty")).not.toBeInTheDocument();
  });

  it("adds an item through the Autocomplete, calling onChange with the full new array", async () => {
    const onChange = jest.fn();
    renderWithTheme(<Harness initial={[OPTIONS[0]]} onChange={onChange} />);

    openAutocomplete();
    // Already-selected items are not offered again.
    const listbox = await screen.findByRole("listbox");
    expect(within(listbox).queryByText("OpenAI")).not.toBeInTheDocument();
    fireEvent.click(within(listbox).getByText("Anthropic"));

    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith([OPTIONS[0], OPTIONS[1]]);
    // The chip is there immediately, with no "+" step in between.
    expect(screen.getByLabelText("Remove Anthropic")).toBeInTheDocument();
    // The search box is cleared for the next add.
    expect(screen.getByRole("combobox", { name: "Add LLM" })).toHaveValue("");
  });

  it("filters options as the user types", async () => {
    renderWithTheme(<Harness />);

    const input = openAutocomplete();
    fireEvent.change(input, { target: { value: "mis" } });

    const listbox = await screen.findByRole("listbox");
    expect(within(listbox).getByText("Mistral")).toBeInTheDocument();
    expect(within(listbox).queryByText("OpenAI")).not.toBeInTheDocument();
  });

  it("removes an item via the chip delete icon", () => {
    const onChange = jest.fn();
    renderWithTheme(<Harness initial={[OPTIONS[0], OPTIONS[1]]} onChange={onChange} />);

    fireEvent.click(screen.getByLabelText("Remove OpenAI"));

    expect(onChange).toHaveBeenCalledWith([OPTIONS[1]]);
    expect(screen.queryByLabelText("Remove OpenAI")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Remove Anthropic")).toBeInTheDocument();
  });

  it("uses getOptionLabel and shows secondary text as an option line", async () => {
    renderWithTheme(
      <Harness
        initial={[{ id: "9", attributes: { name: "Nested" } }]}
        options={[{ id: "10", attributes: { name: "Other" }, mail: "o@x" }]}
        getOptionLabel={(item) => item.attributes.name}
        getOptionSecondary={(item) => item.mail}
      />
    );

    expect(screen.getByLabelText("Remove Nested")).toBeInTheDocument();
    openAutocomplete();
    const listbox = await screen.findByRole("listbox");
    expect(within(listbox).getByText("Other")).toBeInTheDocument();
    expect(within(listbox).getByText("o@x")).toBeInTheDocument();
  });

  it("respects a custom idField", () => {
    const onChange = jest.fn();
    renderWithTheme(
      <Harness
        idField="value"
        initial={[{ value: "a", name: "Alpha" }, { value: "b", name: "Beta" }]}
        options={[{ value: "a", name: "Alpha" }, { value: "b", name: "Beta" }]}
        onChange={onChange}
      />
    );

    expect(screen.getByText("Alpha").closest(".MuiChip-root")).toHaveAttribute("data-item-id", "a");
    fireEvent.click(screen.getByLabelText("Remove Alpha"));
    expect(onChange).toHaveBeenCalledWith([{ value: "b", name: "Beta" }]);
  });

  it("disables the add control and the chip delete when disabled", () => {
    renderWithTheme(<Harness initial={[OPTIONS[0]]} disabled />);

    expect(screen.getByRole("combobox", { name: "Add LLM" })).toBeDisabled();
    expect(screen.queryByLabelText("Remove OpenAI")).not.toBeInTheDocument();
    expect(screen.getByText("OpenAI")).toBeInTheDocument();
  });

  it("shows helperText and the error state", () => {
    renderWithTheme(<Harness error helperText="Pick at least one" />);
    expect(screen.getByText("Pick at least one")).toBeInTheDocument();
    expect(screen.getByText("Pick at least one")).toHaveClass("Mui-error");
  });
});

describe("RelationshipPicker (dual)", () => {
  const USERS = [
    { id: "u1", attributes: { name: "Ada", email: "ada@example.com" } },
    { id: "u2", attributes: { name: "Grace", email: "grace@example.com" } },
    { id: "u3", attributes: { name: "Linus", email: "linus@example.com" } },
  ];
  const nameOf = (u) => u.attributes.name;
  const emailOf = (u) => u.attributes.email;

  const DualHarness = ({ initial = [], onChange, ...props }) => {
    const [value, setValue] = useState(initial);
    return (
      <RelationshipPicker
        variant="dual"
        label="Team members"
        itemLabel="user"
        value={value}
        onChange={(next) => {
          setValue(next);
          onChange?.(next);
        }}
        getOptionLabel={nameOf}
        getOptionSecondary={emailOf}
        {...props}
      />
    );
  };

  it("renders the transfer list with the commit caption and per-row aria-labels", () => {
    renderWithTheme(<DualHarness initial={[USERS[0]]} options={USERS} />);

    expect(screen.getByTestId("relationship-picker")).toHaveAttribute("data-variant", "dual");
    expect(screen.getByTestId("relationship-picker-caption")).toHaveTextContent(COMMIT_CAPTION);
    expect(screen.getByText("Current users")).toBeInTheDocument();
    expect(screen.getByText("Add users")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove Ada" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add Grace" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add Linus" })).toBeInTheDocument();
    // Selected users are not offered on the right.
    expect(screen.queryByRole("button", { name: "Add Ada" })).not.toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Add user" })).toBeInTheDocument();
  });

  it("adds and removes with the row buttons, calling onChange with the full array", () => {
    const onChange = jest.fn();
    renderWithTheme(<DualHarness initial={[USERS[0]]} options={USERS} onChange={onChange} />);

    fireEvent.click(screen.getByRole("button", { name: "Add Grace" }));
    expect(onChange).toHaveBeenLastCalledWith([USERS[0], USERS[1]]);
    expect(screen.getByRole("button", { name: "Remove Grace" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Remove Ada" }));
    expect(onChange).toHaveBeenLastCalledWith([USERS[1]]);
    expect(screen.getByRole("button", { name: "Add Ada" })).toBeInTheDocument();
  });

  it("filters the client-side options with the search box", () => {
    renderWithTheme(<DualHarness options={USERS} />);

    fireEvent.change(screen.getByRole("textbox", { name: "Add user" }), {
      target: { value: "grace@" },
    });

    expect(screen.getByRole("button", { name: "Add Grace" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add Ada" })).not.toBeInTheDocument();
  });

  it("disables the row buttons when disabled", () => {
    renderWithTheme(<DualHarness initial={[USERS[0]]} options={USERS} disabled />);
    expect(screen.getByRole("button", { name: "Remove Ada" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Add Grace" })).toBeDisabled();
  });

  describe("with an async paged source", () => {
    beforeEach(() => {
      jest.useFakeTimers();
    });
    afterEach(() => {
      jest.useRealTimers();
    });

    const pages = {
      1: { items: [USERS[0], USERS[1]], hasMore: true },
      2: { items: [USERS[2]], hasMore: false },
    };

    it("loads the first page, ignores options, pages on load-more and debounces search", async () => {
      const search = jest.fn(async (term, page) => {
        if (term === "lin") return { items: [USERS[2]], hasMore: false };
        return pages[page] || { items: [], hasMore: false };
      });

      renderWithTheme(
        <DualHarness
          options={[{ id: "ignored", attributes: { name: "Ignored", email: "" } }]}
          source={{ search }}
        />
      );

      await waitFor(() => expect(search).toHaveBeenCalledWith("", 1));
      expect(await screen.findByRole("button", { name: "Add Ada" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Add Grace" })).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Add Ignored" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Add Linus" })).not.toBeInTheDocument();

      // Infinite scroll: reaching the bottom of the right pane asks for page 2.
      const scroller = screen.getByRole("button", { name: "Add Ada" }).closest("table").parentElement;
      Object.defineProperty(scroller, "scrollHeight", { configurable: true, value: 400 });
      Object.defineProperty(scroller, "clientHeight", { configurable: true, value: 400 });
      Object.defineProperty(scroller, "scrollTop", { configurable: true, value: 0 });
      fireEvent.scroll(scroller);

      await waitFor(() => expect(search).toHaveBeenCalledWith("", 2));
      expect(await screen.findByRole("button", { name: "Add Linus" })).toBeInTheDocument();
      // Page 1 rows are kept (appended, not replaced).
      expect(screen.getByRole("button", { name: "Add Ada" })).toBeInTheDocument();

      // Search is debounced: typing does not call the source until the timer fires.
      fireEvent.change(screen.getByRole("textbox", { name: "Add user" }), {
        target: { value: "lin" },
      });
      expect(search).not.toHaveBeenCalledWith("lin", 1);
      act(() => {
        jest.advanceTimersByTime(300);
      });
      await waitFor(() => expect(search).toHaveBeenCalledWith("lin", 1));
      expect(await screen.findByRole("button", { name: "Add Linus" })).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Add Ada" })).not.toBeInTheDocument();
    });

    it("adds from a paged source and surfaces a removed item back on the right", async () => {
      const onChange = jest.fn();
      const search = jest.fn(async () => ({ items: [USERS[1]], hasMore: false }));

      renderWithTheme(
        <DualHarness initial={[USERS[0]]} source={{ search }} onChange={onChange} />
      );

      fireEvent.click(await screen.findByRole("button", { name: "Add Grace" }));
      expect(onChange).toHaveBeenLastCalledWith([USERS[0], USERS[1]]);
      expect(screen.queryByRole("button", { name: "Add Grace" })).not.toBeInTheDocument();

      // Ada was never on a loaded page; removing her still shows her as available.
      fireEvent.click(screen.getByRole("button", { name: "Remove Ada" }));
      expect(onChange).toHaveBeenLastCalledWith([USERS[1]]);
      expect(screen.getByRole("button", { name: "Add Ada" })).toBeInTheDocument();
      // No API is ever called by the picker itself for add/remove.
      expect(search).toHaveBeenCalledTimes(1);
    });
  });
});
