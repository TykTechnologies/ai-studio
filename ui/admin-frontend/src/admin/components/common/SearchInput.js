import React, { memo, useCallback } from "react";
import { InputAdornment, IconButton } from "@mui/material";
import SearchIcon from "@mui/icons-material/Search";
import ClearIcon from "@mui/icons-material/Clear";
import { StyledTextField } from "../../styles/sharedStyles";

const SearchInput = memo(({
  value,
  onChange,
  placeholder = "Search...",
  disabled = false,
}) => {
  const handleChange = useCallback((event) => {
    onChange(event.target.value);
  }, [onChange]);

  const handleClear = useCallback(() => {
    onChange("");
  }, [onChange]);

  return (
    <StyledTextField
      placeholder={placeholder}
      variant="outlined"
      fullWidth
      value={value}
      onChange={handleChange}
      disabled={disabled}
      InputProps={{
        startAdornment: (
          <InputAdornment position="start">
            <SearchIcon color="action" />
          </InputAdornment>
        ),
        endAdornment: value ? (
          <InputAdornment position="end">
            <IconButton
              size="small"
              onClick={handleClear}
              disabled={disabled}
              aria-label="Clear search"
            >
              <ClearIcon fontSize="small" />
            </IconButton>
          </InputAdornment>
        ) : null,
      }}
    />
  );
});

SearchInput.displayName = "SearchInput";

export default SearchInput;
