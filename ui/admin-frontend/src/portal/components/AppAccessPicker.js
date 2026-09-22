import React, { useEffect, useMemo, useState } from "react";
import {
  Box,
  IconButton,
  List,
  ListItem,
  ListItemButton,
  ListItemText,
  Tab,
  Tabs,
  Typography,
  useMediaQuery,
} from "@mui/material";
import { useTheme } from "@mui/material/styles";
import AddIcon from "@mui/icons-material/Add";
import CheckIcon from "@mui/icons-material/Check";
import CloseIcon from "@mui/icons-material/Close";
import Icon from "../../components/common/Icon";
import SearchInput from "../../admin/components/common/SearchInput";

/**
 * The App builder's access model: one group per asset type an App can be
 * granted (LLM providers, data sources, tools, MCP servers, and each plugin
 * resource type), and one selection map of group key -> selected ids.
 *
 *   group:     { key, label, itemLabel, icon, helperText?, options: [{ id, name, secondary? }] }
 *   selection: { [group.key]: [option.id, ...] }
 *
 * AccessPicker browses the groups (a type rail and a fixed-height list with
 * a "+" per item); RequestedAccessList shows what has been picked, grouped by
 * type, and is reused read-only on the submission summary.
 */

// Lists longer than this get a search box.
const SEARCH_THRESHOLD = 6;

const selectedIdsFor = (selection, key) => selection[key] || [];

/** The selection resolved to options, grouped, in group order. */
export const resolveSelection = (groups, selection) =>
  groups
    .map((group) => {
      const ids = selectedIdsFor(selection, group.key);
      const byId = new Map(group.options.map((opt) => [opt.id, opt]));
      return { group, items: ids.map((id) => byId.get(id)).filter(Boolean) };
    })
    .filter(({ items }) => items.length > 0);

export const selectionCount = (selection) =>
  Object.values(selection).reduce((total, ids) => total + ids.length, 0);

const GroupIcon = ({ name, ...props }) => <Icon name={name || "puzzle-piece"} fontSize="small" {...props} />;

export const AccessPicker = ({ groups, selection, onToggle, initialGroupKey }) => {
  const theme = useTheme();
  const narrow = useMediaQuery(theme.breakpoints.down("sm"));
  const [activeKey, setActiveKey] = useState(initialGroupKey || groups[0]?.key);
  const [query, setQuery] = useState("");

  // Keep the active tab valid if the group list changes under it.
  useEffect(() => {
    if (!groups.some((g) => g.key === activeKey)) setActiveKey(groups[0]?.key);
  }, [groups, activeKey]);

  const active = groups.find((g) => g.key === activeKey) || groups[0];
  const selectedIds = useMemo(
    () => new Set(active ? selectedIdsFor(selection, active.key) : []),
    [selection, active],
  );
  const visibleOptions = useMemo(() => {
    if (!active) return [];
    const term = query.trim().toLowerCase();
    if (!term) return active.options;
    return active.options.filter((opt) =>
      `${opt.name} ${opt.secondary || ""}`.toLowerCase().includes(term),
    );
  }, [active, query]);

  if (!active) return null;

  const switchGroup = (_, key) => {
    setActiveKey(key);
    setQuery("");
  };

  return (
    <Box
      data-testid="app-access-picker"
      sx={{
        display: "flex",
        flexDirection: narrow ? "column" : "row",
        border: 1,
        borderColor: "border.neutralDefault",
        borderRadius: 2,
        overflow: "hidden",
        bgcolor: "background.paper",
      }}
    >
      <Tabs
        orientation={narrow ? "horizontal" : "vertical"}
        variant="scrollable"
        scrollButtons={narrow ? "auto" : false}
        value={active.key}
        onChange={switchGroup}
        aria-label="Asset types"
        sx={{
          flexShrink: 0,
          width: narrow ? "100%" : 220,
          bgcolor: "background.surfaceNeutralHover",
          borderRight: narrow ? 0 : 1,
          borderBottom: narrow ? 1 : 0,
          borderColor: "border.neutralDefault",
          "& .MuiTabs-indicator": { left: narrow ? undefined : 0 },
        }}
      >
        {groups.map((group) => {
          const count = selectedIdsFor(selection, group.key).length;
          return (
            <Tab
              key={group.key}
              value={group.key}
              id={`app-access-tab-${group.key}`}
              aria-controls="app-access-panel"
              icon={<GroupIcon name={group.icon} />}
              iconPosition="start"
              label={
                <Box sx={{ display: "flex", alignItems: "center", width: "100%", gap: 1 }}>
                  <Box component="span" data-testid="app-access-tab-label" sx={{ flex: 1, textAlign: "left" }}>
                    {group.label}
                  </Box>
                  {count > 0 && (
                    <Box
                      component="span"
                      aria-label={`${count} added`}
                      sx={{
                        minWidth: 20,
                        height: 20,
                        px: 0.75,
                        borderRadius: 10,
                        bgcolor: "background.buttonPrimaryDefault",
                        color: "common.white",
                        fontSize: 12,
                        lineHeight: "20px",
                        textAlign: "center",
                      }}
                    >
                      {count}
                    </Box>
                  )}
                </Box>
              }
              sx={{
                textTransform: "none",
                justifyContent: "flex-start",
                minHeight: 48,
                px: 2,
                typography: "bodyLargeMedium",
                color: "text.primary",
                "&.Mui-selected": { bgcolor: "background.paper", color: "text.primary" },
              }}
            />
          );
        })}
      </Tabs>

      <Box
        role="tabpanel"
        id="app-access-panel"
        aria-labelledby={`app-access-tab-${active.key}`}
        sx={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column" }}
      >
        {active.options.length > SEARCH_THRESHOLD && (
          <Box sx={{ p: 1.5, borderBottom: 1, borderColor: "border.neutralDefaultSubdued" }}>
            <SearchInput
              value={query}
              onChange={setQuery}
              placeholder="Search by name"
            />
          </Box>
        )}
        <List
          dense
          disablePadding
          aria-label={active.label}
          sx={{ height: 320, overflowY: "auto" }}
          data-testid="app-access-options"
        >
          {visibleOptions.length === 0 && (
            <ListItem>
              <ListItemText
                primary={`Nothing matches "${query}".`}
                primaryTypographyProps={{ color: "text.secondary" }}
              />
            </ListItem>
          )}
          {visibleOptions.map((opt) => {
            const added = selectedIds.has(opt.id);
            return (
              <ListItem key={opt.id} disablePadding divider>
                <ListItemButton
                  onClick={() => onToggle(active.key, opt.id)}
                  selected={added}
                  aria-label={added ? `Remove ${opt.name}` : `Add ${opt.name}`}
                  aria-pressed={added}
                  sx={{ py: 1 }}
                >
                  <ListItemText
                    primary={opt.name}
                    secondary={opt.secondary || null}
                    primaryTypographyProps={{ variant: "bodyLargeMedium" }}
                    secondaryTypographyProps={{ noWrap: true }}
                    sx={{ minWidth: 0, mr: 1 }}
                  />
                  {added ? (
                    <CheckIcon fontSize="small" color="success" aria-hidden />
                  ) : (
                    <AddIcon fontSize="small" color="action" aria-hidden />
                  )}
                </ListItemButton>
              </ListItem>
            );
          })}
        </List>
        {active.helperText && (
          <Typography
            variant="bodySmallDefault"
            color="text.secondary"
            component="p"
            sx={{ px: 2, py: 1.5, borderTop: 1, borderColor: "border.neutralDefaultSubdued" }}
          >
            {active.helperText}
          </Typography>
        )}
      </Box>
    </Box>
  );
};

/**
 * What the App asks for, grouped by type. With onRemove each item has a
 * remove button; without it the list is a read-only summary.
 */
export const RequestedAccessList = ({ groups, selection, onRemove, emptyText }) => {
  const resolved = resolveSelection(groups, selection);

  if (resolved.length === 0) {
    return (
      <Box
        data-testid="requested-access-empty"
        sx={{
          border: 1,
          borderStyle: "dashed",
          borderColor: "border.neutralDefault",
          borderRadius: 2,
          p: 2,
        }}
      >
        <Typography variant="bodyMediumDefault" color="text.secondary">
          {emptyText}
        </Typography>
      </Box>
    );
  }

  return (
    <Box data-testid="requested-access" sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
      {resolved.map(({ group, items }) => (
        <Box key={group.key} data-testid={`requested-access-${group.key}`}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5, color: "text.secondary" }}>
            <GroupIcon name={group.icon} />
            <Typography variant="bodyMediumSemiBold" component="h3">
              {group.label}
            </Typography>
          </Box>
          <List
            dense
            disablePadding
            sx={{ border: 1, borderColor: "border.neutralDefault", borderRadius: 2 }}
          >
            {items.map((opt, index) => (
              <ListItem
                key={opt.id}
                divider={index < items.length - 1}
                data-testid="requested-access-item"
                secondaryAction={
                  onRemove && (
                    <IconButton
                      edge="end"
                      size="small"
                      aria-label={`Remove ${opt.name}`}
                      onClick={() => onRemove(group.key, opt.id)}
                    >
                      <CloseIcon fontSize="small" />
                    </IconButton>
                  )
                }
              >
                <ListItemText
                  primary={opt.name}
                  secondary={opt.secondary || null}
                  primaryTypographyProps={{ variant: "bodyLargeMedium" }}
                  secondaryTypographyProps={{ noWrap: true }}
                />
              </ListItem>
            ))}
          </List>
        </Box>
      ))}
    </Box>
  );
};
