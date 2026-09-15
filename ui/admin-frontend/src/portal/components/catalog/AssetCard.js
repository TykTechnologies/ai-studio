import React from "react";
import { useNavigate } from "react-router-dom";
import { Box, Typography, styled } from "@mui/material";
import AssetAvatar from "./AssetAvatar";
import AssetTypeChip from "./AssetTypeChip";
import PrivacyLevelChip from "../../../admin/components/common/privacy/PrivacyLevelChip";
import CommunityBadge from "../../../admin/components/submissions/CommunityBadge";
import GovernedMetadataBadges from "../GovernedMetadataBadges";
import { PrimaryButton, SecondaryOutlineButton } from "../../../admin/styles/sharedStyles";
import { relativeTime } from "../../../admin/components/notifications/notificationPresentation";
import {
  buildActionLabel,
  buildAppPath,
  detailPath,
  isAppGranted,
  itemKey,
  kindLabel,
  kindLogo,
  secondaryActionLabel,
  secondaryActionPath,
} from "../../utils/catalog";

const CardRoot = styled(Box)(({ theme }) => ({
  border: `1px solid ${theme.palette.border.neutralDefault}`,
  borderRadius: "8px",
  backgroundColor: theme.palette.background.paper,
  boxShadow: "4px 4px 8px 0px rgba(9, 9, 35, 0.06)",
  display: "flex",
  flexDirection: "column",
  height: "100%",
  boxSizing: "border-box",
  cursor: "pointer",
  transition: "border-color 120ms ease, box-shadow 120ms ease",
  "&:hover, &:focus-visible": {
    borderColor: theme.palette.border.neutralHovered,
    boxShadow: "4px 4px 12px 0px rgba(9, 9, 35, 0.12)",
    outline: "none",
  },
}));

const CardBody = styled(Box)(({ theme }) => ({
  padding: theme.spacing(2),
  display: "flex",
  flexDirection: "column",
  gap: theme.spacing(1.5),
  flex: 1,
  minWidth: 0,
}));

const CardFooter = styled(Box)(({ theme }) => ({
  padding: theme.spacing(1.5, 2),
  borderTop: `1px solid ${theme.palette.border.neutralDefaultSubdued}`,
  display: "flex",
  justifyContent: "flex-end",
  alignItems: "center",
  gap: theme.spacing(1),
  flexWrap: "wrap",
}));

const Clamp = styled(Typography)({
  display: "-webkit-box",
  WebkitLineClamp: 2,
  WebkitBoxOrient: "vertical",
  overflow: "hidden",
  wordBreak: "break-word",
});

/**
 * One card for every asset type in the portal (LLM provider, data source,
 * tool, plugin resource). The whole card opens the detail page; the primary
 * action goes straight to the app builder with the asset preselected.
 */
const AssetCard = ({ item, showType = true, compact = false }) => {
  const navigate = useNavigate();
  const attrs = item?.attributes || {};
  const kind = kindLabel(item);
  const logo = kindLogo(item);
  const detail = detailPath(item);
  const added = attrs.created_at ? relativeTime(attrs.created_at) : "";

  const open = () => navigate(detail);
  const onKeyDown = (event) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      open();
    }
  };

  return (
    <CardRoot
      component="article"
      role="link"
      tabIndex={0}
      aria-label={attrs.name}
      data-testid="asset-card"
      data-asset-key={itemKey(item)}
      onClick={open}
      onKeyDown={onKeyDown}
    >
      <CardBody>
        <Box sx={{ display: "flex", gap: 1.5, alignItems: "flex-start", minWidth: 0 }}>
          <AssetAvatar name={attrs.name} seed={itemKey(item)} logoUrl={attrs.logo_url} size={compact ? 44 : 52} />
          <Box sx={{ minWidth: 0, flex: 1 }}>
            <Typography
              variant="bodyLargeBold"
              component="h3"
              sx={{ m: 0, lineHeight: 1.3, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
              title={attrs.name}
            >
              {attrs.name}
            </Typography>
            <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, mt: 0.5, flexWrap: "wrap" }}>
              {showType && <AssetTypeChip item={item} />}
              {kind && (
                <Box sx={{ display: "inline-flex", alignItems: "center", gap: 0.5, minWidth: 0 }}>
                  {logo && (
                    <img
                      src={logo}
                      alt=""
                      style={{ width: 14, height: 14, objectFit: "contain" }}
                      onError={(event) => {
                        event.currentTarget.style.display = "none";
                      }}
                    />
                  )}
                  <Typography variant="bodySmallDefault" color="text.defaultSubdued" noWrap>
                    {kind}
                  </Typography>
                </Box>
              )}
            </Box>
          </Box>
        </Box>

        {attrs.short_description ? (
          <Clamp variant="bodyMediumDefault" color="text.defaultSubdued">
            {attrs.short_description}
          </Clamp>
        ) : (
          <Typography variant="bodyMediumDefault" color="text.neutralDisabled">
            No description provided.
          </Typography>
        )}

        {!compact && <GovernedMetadataBadges items={item?.governed_metadata} />}

        <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap", mt: "auto" }}>
          {attrs.privacy_score !== null && attrs.privacy_score !== undefined && (
            <PrivacyLevelChip score={attrs.privacy_score} />
          )}
          <CommunityBadge show={Boolean(attrs.community_submitted)} />
          {added && (
            <Typography variant="bodySmallDefault" color="text.neutralDisabled" sx={{ ml: "auto" }}>
              Added {added}
            </Typography>
          )}
        </Box>
      </CardBody>

      <CardFooter onClick={(event) => event.stopPropagation()}>
        <SecondaryOutlineButton size="small" onClick={open} data-testid="asset-card-details">
          Details
        </SecondaryOutlineButton>
        {/* "Build app" only when an App credential is what grants access;
            otherwise the providing plugin's own page is the way in. */}
        {isAppGranted(item) ? (
          <PrimaryButton
            size="small"
            sx={{ padding: "2px 12px" }}
            onClick={() => navigate(buildAppPath(item))}
            data-testid="asset-card-build"
          >
            {buildActionLabel(item)}
          </PrimaryButton>
        ) : (
          secondaryActionPath(item) && (
            <SecondaryOutlineButton
              size="small"
              onClick={() => navigate(secondaryActionPath(item))}
              data-testid="asset-card-view"
            >
              {secondaryActionLabel(item)}
            </SecondaryOutlineButton>
          )
        )}
      </CardFooter>
    </CardRoot>
  );
};

export default AssetCard;
