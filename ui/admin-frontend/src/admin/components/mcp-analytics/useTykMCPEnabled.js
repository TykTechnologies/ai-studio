import { useEffect, useState } from "react";
import apiClient from "../../utils/apiClient";

// useTykMCPEnabled reports whether Tyk MCP servers exist on this edition and
// are switched on (the same /tyk-mcp/status the connection pages read). MCP
// analytics only come from Tyk, so admin views hide their MCP sections
// otherwise. Any error counts as off.
const useTykMCPEnabled = () => {
  const [enabled, setEnabled] = useState(false);
  useEffect(() => {
    let cancelled = false;
    apiClient
      .get("/tyk-mcp/status")
      .then((res) => !cancelled && setEnabled(Boolean(res?.data?.available && res?.data?.enabled)))
      .catch(() => !cancelled && setEnabled(false));
    return () => {
      cancelled = true;
    };
  }, []);
  return enabled;
};

export default useTykMCPEnabled;
