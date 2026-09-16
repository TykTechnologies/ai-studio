import { useState, useCallback } from "react";
import { apiErrorDetail } from "../../../../pages/webhookShared";
import { getTykStatus, listTykConnections, listTykAPIs, getTykAPIDocument } from "../services/toolService";

/**
 * useTykImport holds what the Tyk Dashboard path of the import wizard
 * fetches: the integration status, the saved connections, the APIs of the
 * chosen connection and one API's definition.
 */
export const useTykImport = () => {
  const [status, setStatus] = useState(null);
  const [connections, setConnections] = useState([]);
  const [apis, setApis] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const loadConnections = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const st = await getTykStatus();
      setStatus(st);
      if (st?.available && st?.enabled) {
        setConnections(await listTykConnections());
      } else {
        setConnections([]);
      }
      return st;
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to load Tyk connections"));
      throw err;
    } finally {
      setLoading(false);
    }
  }, []);

  const loadAPIs = useCallback(async (connectionId) => {
    setLoading(true);
    setError("");
    setApis([]);
    try {
      const list = await listTykAPIs(connectionId);
      setApis(list);
      return list;
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to list the Dashboard's APIs"));
      throw err;
    } finally {
      setLoading(false);
    }
  }, []);

  const loadDocument = useCallback(async (connectionId, apiId) => {
    setLoading(true);
    setError("");
    try {
      return await getTykAPIDocument(connectionId, apiId);
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to fetch the API definition"));
      throw err;
    } finally {
      setLoading(false);
    }
  }, []);

  const reset = useCallback(() => {
    setApis([]);
    setError("");
  }, []);

  return { status, connections, apis, loading, error, loadConnections, loadAPIs, loadDocument, reset };
};
