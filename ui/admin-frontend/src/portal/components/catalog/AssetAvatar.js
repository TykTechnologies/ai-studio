import React, { useEffect, useState } from "react";
import { Box } from "@mui/material";
import { avatarColor, initialsFor } from "../../utils/catalog";

/**
 * The portal's stand-in for an asset logo. Administrators and submitters may
 * upload one, but most assets have none, and the generic placeholder image
 * made every card look the same. Instead each asset gets a Gravatar-style
 * tile: its initials on a colour derived from its name, so the same asset
 * always looks the same and different assets can be told apart at a glance.
 * A real logo, when there is one, is shown in its place and the initials
 * take over if the image fails to load.
 */
const AssetAvatar = ({ name, seed, logoUrl, size = 56, sx = {}, ...rest }) => {
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    setFailed(false);
  }, [logoUrl]);

  const colors = avatarColor(seed || name);
  const showLogo = Boolean(logoUrl) && !failed;
  const initials = initialsFor(name);

  return (
    <Box
      data-testid="asset-avatar"
      data-avatar-mode={showLogo ? "logo" : "initials"}
      aria-hidden="true"
      sx={{
        width: size,
        height: size,
        minWidth: size,
        borderRadius: `${Math.round(size * 0.22)}px`,
        background: showLogo ? (theme) => theme.palette.background.paper : colors.background,
        border: showLogo ? (theme) => `1px solid ${theme.palette.border.neutralDefault}` : "none",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        overflow: "hidden",
        flexShrink: 0,
        boxSizing: "border-box",
        ...sx,
      }}
      {...rest}
    >
      {showLogo ? (
        <img
          src={logoUrl}
          alt=""
          data-testid="asset-avatar-logo"
          onError={() => setFailed(true)}
          style={{ width: "78%", height: "78%", objectFit: "contain", display: "block" }}
        />
      ) : (
        <span
          style={{
            color: colors.foreground,
            fontFamily: "Inter-Bold, Inter, sans-serif",
            fontSize: `${Math.round(size * 0.36)}px`,
            lineHeight: 1,
            letterSpacing: "0.02em",
            userSelect: "none",
          }}
        >
          {initials}
        </span>
      )}
    </Box>
  );
};

export default AssetAvatar;
