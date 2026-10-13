import React, { useEffect, useState } from "react";
import { Alert } from "@mui/material";
import apiClient from "../../../utils/apiClient";
import { listAll } from "../../../utils/listAll";
import { formatPrivacyLevel, normalizePrivacyScore } from "./privacyLevels";

/**
 * The highest privacy level among the instance's active LLM providers, from
 * one bounded listing: { loading, highest: { score, name } | null }.
 * highest is null when there are no active providers (or they could not be
 * read).
 */
const useProviderPrivacyCeiling = () => {
  const [state, setState] = useState({ loading: true, highest: null });
  useEffect(() => {
    let cancelled = false;
    listAll(apiClient, "/llms")
      .then((res) => {
        let highest = null;
        (res.data?.data || []).forEach((llm) => {
          const attrs = llm.attributes || {};
          const score = normalizePrivacyScore(attrs.privacy_score);
          if (!attrs.active || score === null) return;
          if (!highest || score > highest.score) highest = { score, name: attrs.name };
        });
        if (!cancelled) setState({ loading: false, highest });
      })
      .catch(() => {
        if (!cancelled) setState({ loading: false, highest: null, failed: true });
      });
    return () => {
      cancelled = true;
    };
  }, []);
  return state;
};

/**
 * What a privacy level means against the providers that exist (#503). A tool
 * or data source can only be used with an LLM provider whose level is at or
 * above its own, so a level above every active provider makes it unusable
 * until a provider is raised. Admin-side only: contributors are not shown
 * provider levels.
 *
 *   - above every active provider: a warning naming the highest level;
 *   - no active providers yet: a neutral hint (nothing to compare against);
 *   - otherwise, or while loading or when the providers cannot be read:
 *     nothing.
 */
const ProviderPrivacyWarning = ({ score, sx }) => {
  const { loading, highest, failed } = useProviderPrivacyCeiling();
  const value = normalizePrivacyScore(score);
  if (loading || failed || value === null) return null;

  if (!highest) {
    return (
      <Alert severity="info" sx={sx} data-testid="provider-privacy-hint">
        There are no active LLM providers yet. Tools and data sources can only be used with providers at or above
        their privacy level, so set your providers' levels with this in mind.
      </Alert>
    );
  }

  if (value > highest.score) {
    return (
      <Alert severity="warning" sx={sx} data-testid="provider-privacy-warning">
        No active LLM provider can use this. The highest provider privacy level is{" "}
        <strong>{formatPrivacyLevel(highest.score)}</strong> ({highest.name}). Lower this level, or raise a provider's
        level.
      </Alert>
    );
  }

  return null;
};

export default ProviderPrivacyWarning;
