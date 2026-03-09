import { useState, useEffect, useCallback } from "react";
import pubClient from "../../admin/utils/pubClient";
import { executeRecaptchaV3 } from "../components/CaptchaWidget";

/**
 * Hook that loads CAPTCHA config from the backend and manages token state.
 *
 * Returns:
 *   captchaConfig  – { provider, site_key } or null if captcha is disabled
 *   captchaToken   – current token string (set by widget callback or getToken)
 *   setCaptchaToken – setter for explicit widget callbacks
 *   getToken       – async function that returns a fresh token (needed for reCAPTCHA v3)
 */
const useCaptcha = () => {
  const [captchaConfig, setCaptchaConfig] = useState(null);
  const [captchaToken, setCaptchaToken] = useState("");

  useEffect(() => {
    const fetchConfig = async () => {
      try {
        const response = await pubClient.get("/auth/config");
        if (response.data?.captcha) {
          setCaptchaConfig(response.data.captcha);
        }
      } catch {
        // If config fetch fails, captcha stays disabled
      }
    };
    fetchConfig();
  }, []);

  // getToken returns the current token for widget-based providers,
  // or executes a fresh token request for reCAPTCHA v3.
  const getToken = useCallback(async () => {
    if (!captchaConfig) return "";
    if (captchaConfig.provider === "recaptcha_v3") {
      return executeRecaptchaV3(captchaConfig.site_key);
    }
    return captchaToken;
  }, [captchaConfig, captchaToken]);

  return { captchaConfig, captchaToken, setCaptchaToken, getToken };
};

export default useCaptcha;
