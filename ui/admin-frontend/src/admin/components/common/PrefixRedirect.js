import React from "react";
import { Navigate, useLocation, useParams } from "react-router-dom";

/**
 * For a renamed admin route mounted at "<old>/*": forwards to the same place
 * under /admin/<to>, keeping the rest of the path, the query and the hash.
 */
const PrefixRedirect = ({ to }) => {
  const { "*": rest = "" } = useParams();
  const { search, hash } = useLocation();
  const suffix = rest ? `/${rest}` : "";
  return <Navigate to={`/admin/${to}${suffix}${search}${hash}`} replace />;
};

export default PrefixRedirect;
