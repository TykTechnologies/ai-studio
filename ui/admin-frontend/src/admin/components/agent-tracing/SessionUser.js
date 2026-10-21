import React from "react";
import { Tooltip } from "@mui/material";

// Who a session belongs to: the Studio user's name, else the end user the
// client declared, else the Studio user id. For a viewer without
// trace-content read the API returns the end user as a 32-hex pseudonym
// (jyx.174); it is labelled as such so it is not mistaken for an id the
// client sent, and it is the same pseudonym exports carry.
const SessionUser = ({ session, masked }) => {
  const s = session || {};
  if (s.user_name) return s.user_name;
  if (s.end_user) {
    if (!masked) return s.end_user;
    return (
      <Tooltip title="Pseudonym: your role sees end users as stable pseudonyms, not the ids clients sent">
        <span data-testid="end-user-pseudonym">{s.end_user} (pseudonym)</span>
      </Tooltip>
    );
  }
  return s.user_id ? `User ${s.user_id}` : "—";
};

export default SessionUser;
