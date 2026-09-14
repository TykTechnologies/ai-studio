import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  Autocomplete,
  Box,
  Chip,
  FormHelperText,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import CloseIcon from "@mui/icons-material/Close";
import TransferList from "../transfer-list/TransferList";

/**
 * RelationshipPicker -- the one way to say "which X belong to Y".
 *
 * Replaces the five relationship idioms the UX review found (catalogue
 * "pick, +, then Save", user-form "+ commits immediately", the team dual
 * table, the app multi-select, the portal builder "pick then Add"). It is a
 * controlled component: `value` is the full selected array, `onChange` is
 * called with the full new array, and it never calls an API itself. Every
 * variant states the same commit rule under the control:
 * "Changes apply when you save this form."
 *
 * - `compact` (default): selected items as deletable chips, plus an
 *   Autocomplete over `options` minus the selection. Choosing an option adds
 *   it immediately to `value`; there is no "+" button and no pending state.
 * - `dual`: the TransferList (selected on the left with Remove, available on
 *   the right with a search box). With `source` the right side is paged from
 *   `source.search(term, page)` (debounced, infinite scroll); without it the
 *   right side is `options` minus the selection, filtered client-side.
 */

const SEARCH_DEBOUNCE_MS = 300;

export const COMMIT_CAPTION = "Changes apply when you save this form.";

const pluralize = (word) => (word ? `${word}s` : "items");

const defaultGetOptionLabel = (item) => item?.name ?? "";

const RelationshipPicker = ({
  label,
  itemLabel = "item",
  variant = "compact",
  value = [],
  onChange,
  options = [],
  source,
  getOptionLabel = defaultGetOptionLabel,
  getOptionSecondary,
  columns,
  idField = "id",
  disabled = false,
  error = false,
  helperText = "",
  emptyText,
  loading = false,
}) => {
  const selected = useMemo(() => value || [], [value]);
  const plural = pluralize(itemLabel);
  const resolvedEmptyText = emptyText ?? `No ${plural} selected`;
  const isDual = variant === "dual";
  const hasSource = isDual && Boolean(source?.search);

  const selectedIds = useMemo(
    () => new Set(selected.map((item) => String(item?.[idField]))),
    [selected, idField],
  );

  const nameOf = useCallback(
    (item) => String(getOptionLabel(item) ?? ""),
    [getOptionLabel],
  );

  const secondaryOf = useCallback(
    (item) => (getOptionSecondary ? getOptionSecondary(item) : undefined),
    [getOptionSecondary],
  );

  // ---- selection helpers (never mutate `value`) ---------------------------
  const emit = useCallback(
    (next) => {
      onChange?.(next);
    },
    [onChange],
  );

  const addItem = useCallback(
    (item) => {
      if (!item || selectedIds.has(String(item[idField]))) return;
      emit([...selected, item]);
    },
    [emit, selected, selectedIds, idField],
  );

  const removeItem = useCallback(
    (item) => {
      emit(selected.filter((s) => String(s[idField]) !== String(item[idField])));
    },
    [emit, selected, idField],
  );

  // ---- dual: search state -------------------------------------------------
  const [searchTerm, setSearchTerm] = useState("");
  const [sourceItems, setSourceItems] = useState([]);
  const [sourcePage, setSourcePage] = useState(1);
  const [sourceHasMore, setSourceHasMore] = useState(false);
  const [isSearching, setIsSearching] = useState(false);
  const [isLoadingMore, setIsLoadingMore] = useState(false);

  // Consumers may pass an inline `source` object; keep it in a ref so its
  // identity never re-triggers the fetch effects.
  const sourceRef = useRef(source);
  sourceRef.current = source;
  // Monotonic request id so a slow earlier response cannot overwrite a newer one.
  const requestRef = useRef(0);

  const runSearch = useCallback(async (term, page, append) => {
    const search = sourceRef.current?.search;
    if (!search) return;
    const requestId = ++requestRef.current;
    if (append) setIsLoadingMore(true);
    else setIsSearching(true);
    try {
      const result = (await search(term, page)) || {};
      if (requestId !== requestRef.current) return;
      const items = Array.isArray(result.items) ? result.items : [];
      setSourceItems((prev) => {
        if (!append) return items;
        const seen = new Set(prev.map((i) => String(i?.[idField])));
        return [...prev, ...items.filter((i) => !seen.has(String(i?.[idField])))];
      });
      setSourceHasMore(Boolean(result.hasMore));
      setSourcePage(page);
    } catch (err) {
      if (requestId !== requestRef.current) return;
      if (!append) setSourceItems([]);
      setSourceHasMore(false);
    } finally {
      if (requestId === requestRef.current) {
        setIsSearching(false);
        setIsLoadingMore(false);
      }
    }
  }, [idField]);

  // Initial page load, then a debounced reload whenever the term changes.
  const isFirstSearchRef = useRef(true);
  useEffect(() => {
    if (!hasSource) return undefined;
    if (isFirstSearchRef.current) {
      isFirstSearchRef.current = false;
      runSearch("", 1, false);
      return undefined;
    }
    const handle = setTimeout(() => runSearch(searchTerm, 1, false), SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(handle);
  }, [hasSource, searchTerm, runSearch]);

  const handleLoadMore = useCallback(() => {
    if (!hasSource || !sourceHasMore || isLoadingMore || isSearching) return;
    runSearch(searchTerm, sourcePage + 1, true);
  }, [hasSource, sourceHasMore, isLoadingMore, isSearching, runSearch, searchTerm, sourcePage]);

  const matchesTerm = useCallback(
    (item) => {
      const term = searchTerm.trim().toLowerCase();
      if (!term) return true;
      const haystack = `${nameOf(item)} ${secondaryOf(item) ?? ""}`.toLowerCase();
      return haystack.includes(term);
    },
    [searchTerm, nameOf, secondaryOf],
  );

  const availableItems = useMemo(() => {
    const pool = hasSource ? sourceItems : options || [];
    const notSelected = pool.filter((item) => item && !selectedIds.has(String(item[idField])));
    return hasSource ? notSelected : notSelected.filter(matchesTerm);
  }, [hasSource, sourceItems, options, selectedIds, idField, matchesTerm]);

  const handleDualRemove = useCallback(
    (item) => {
      removeItem(item);
      // With a paged source the removed item may not be on the loaded pages;
      // surface it at the top of the available list so the change is visible.
      if (hasSource) {
        setSourceItems((prev) => [
          item,
          ...prev.filter((i) => String(i?.[idField]) !== String(item[idField])),
        ]);
      }
    },
    [removeItem, hasSource, idField],
  );

  const dualColumns = useMemo(() => {
    if (columns && columns.length > 0) return columns;
    return [
      {
        field: "name",
        headerName: itemLabel.charAt(0).toUpperCase() + itemLabel.slice(1),
        width: "75%",
        renderCell: (item) => {
          const secondary = secondaryOf(item);
          return (
            <Box sx={{ display: "flex", flexDirection: "column", minWidth: 0 }}>
              <Typography variant="bodyMediumMedium" color="text.primary" noWrap>
                {nameOf(item)}
              </Typography>
              {secondary && (
                <Typography variant="bodySmallDefault" color="text.defaultSubdued" noWrap>
                  {secondary}
                </Typography>
              )}
            </Box>
          );
        },
      },
    ];
  }, [columns, itemLabel, nameOf, secondaryOf]);

  // ---- compact: autocomplete state ----------------------------------------
  const [inputValue, setInputValue] = useState("");

  const handleAutocompleteChange = useCallback(
    (event, item) => {
      if (item) addItem(item);
      setInputValue("");
    },
    [addItem],
  );

  const renderCaption = () => (
    <>
      {helperText && (
        <FormHelperText error={error} sx={{ mx: 0 }}>
          {helperText}
        </FormHelperText>
      )}
      <Typography
        variant="bodySmallDefault"
        color="text.defaultSubdued"
        data-testid="relationship-picker-caption"
        sx={{ display: "block", mt: 1 }}
      >
        {COMMIT_CAPTION}
      </Typography>
    </>
  );

  return (
    <Box
      data-testid="relationship-picker"
      data-variant={variant}
      role="group"
      aria-label={label}
      sx={{ width: "100%" }}
    >
      {label && (
        <Typography
          variant="headingSmall"
          color="text.primary"
          component="div"
          sx={{ mb: 1 }}
        >
          {label}
        </Typography>
      )}

      {isDual ? (
        <TransferList
          availableItems={availableItems}
          selectedItems={selected}
          columns={dualColumns}
          idField={idField}
          leftTitle={`Current ${plural}`}
          leftSubtitle={`${plural.charAt(0).toUpperCase() + plural.slice(1)} currently selected`}
          rightTitle={`Add ${plural}`}
          rightSubtitle={`Search for ${plural} to add`}
          enableSearch={true}
          searchTerm={searchTerm}
          onSearchTermChange={setSearchTerm}
          searchLabel={`Add ${itemLabel}`}
          searchPlaceholder={`Add ${itemLabel}…`}
          isSearching={hasSource ? isSearching : false}
          onAdd={addItem}
          onRemove={handleDualRemove}
          onLoadMore={handleLoadMore}
          hasMore={hasSource ? sourceHasMore : false}
          isLoadingMore={hasSource ? isLoadingMore || loading : loading}
          itemLabel={itemLabel}
          getItemName={nameOf}
          disabled={disabled}
        />
      ) : (
        <>
          <Box
            data-testid="relationship-picker-selected"
            sx={{ display: "flex", flexWrap: "wrap", gap: 1, mb: 1.5, minHeight: 32 }}
          >
            {selected.length === 0 ? (
              <Typography
                variant="bodyMediumDefault"
                color="text.defaultSubdued"
                data-testid="relationship-picker-empty"
                sx={{ alignSelf: "center" }}
              >
                {resolvedEmptyText}
              </Typography>
            ) : (
              selected.map((item) => {
                const name = nameOf(item);
                const secondary = secondaryOf(item);
                const chip = (
                  <Chip
                    key={item[idField]}
                    label={name}
                    variant="outlined"
                    data-item-id={item[idField]}
                    onDelete={disabled ? undefined : () => removeItem(item)}
                    deleteIcon={
                      // SvgIcon hides itself from the a11y tree by default;
                      // this is the remove control, so expose it as a button.
                      <CloseIcon
                        role="button"
                        aria-hidden={false}
                        aria-label={`Remove ${name}`}
                      />
                    }
                    sx={{ borderRadius: "6px" }}
                  />
                );
                return secondary ? (
                  <Tooltip key={item[idField]} title={secondary} arrow>
                    {chip}
                  </Tooltip>
                ) : (
                  chip
                );
              })
            )}
          </Box>

          <Autocomplete
            options={availableItems}
            value={null}
            inputValue={inputValue}
            onInputChange={(event, next) => setInputValue(next)}
            onChange={handleAutocompleteChange}
            getOptionLabel={nameOf}
            isOptionEqualToValue={(a, b) => String(a?.[idField]) === String(b?.[idField])}
            disabled={disabled}
            loading={loading}
            clearOnBlur
            handleHomeEndKeys
            size="small"
            noOptionsText={`No ${plural} to add`}
            renderOption={(props, option) => {
              const { key, ...rest } = props;
              const secondary = secondaryOf(option);
              return (
                <li key={option[idField]} {...rest} data-option-id={option[idField]}>
                  <Box sx={{ display: "flex", flexDirection: "column", minWidth: 0 }}>
                    <Typography variant="bodyMediumDefault" noWrap>
                      {nameOf(option)}
                    </Typography>
                    {secondary && (
                      <Typography variant="bodySmallDefault" color="text.defaultSubdued" noWrap>
                        {secondary}
                      </Typography>
                    )}
                  </Box>
                </li>
              );
            }}
            renderInput={(params) => (
              <TextField
                {...params}
                label={`Add ${itemLabel}`}
                placeholder={`Add ${itemLabel}…`}
                error={error}
              />
            )}
          />
        </>
      )}

      {renderCaption()}
    </Box>
  );
};

export default RelationshipPicker;
