import React, { useEffect, useRef, useCallback } from "react";
import { Box } from "@mui/material";

const SCRIPT_URLS = {
  recaptcha_v2: "https://www.google.com/recaptcha/api.js?onload=onRecaptchaLoad&render=explicit",
  recaptcha_v3: "https://www.google.com/recaptcha/api.js?render=",
  hcaptcha: "https://js.hcaptcha.com/1/api.js?onload=onHCaptchaLoad&render=explicit",
  turnstile: "https://challenges.cloudflare.com/turnstile/v0/api.js?onload=onTurnstileLoad&render=explicit",
};

function loadScript(src) {
  if (document.querySelector(`script[src="${src}"]`)) return Promise.resolve();
  return new Promise((resolve, reject) => {
    const s = document.createElement("script");
    s.src = src;
    s.async = true;
    s.defer = true;
    s.onload = resolve;
    s.onerror = reject;
    document.head.appendChild(s);
  });
}

const CaptchaWidget = ({ provider, siteKey, instanceUrl, onToken }) => {
  const containerRef = useRef(null);
  const widgetIdRef = useRef(null);
  const readyRef = useRef(false);

  const renderWidget = useCallback(() => {
    if (!containerRef.current || readyRef.current) return;

    switch (provider) {
      case "recaptcha_v2":
        if (window.grecaptcha && window.grecaptcha.render) {
          widgetIdRef.current = window.grecaptcha.render(containerRef.current, {
            sitekey: siteKey,
            callback: onToken,
            "expired-callback": () => onToken(""),
          });
          readyRef.current = true;
        }
        break;
      case "hcaptcha":
        if (window.hcaptcha && window.hcaptcha.render) {
          widgetIdRef.current = window.hcaptcha.render(containerRef.current, {
            sitekey: siteKey,
            callback: onToken,
            "expired-callback": () => onToken(""),
          });
          readyRef.current = true;
        }
        break;
      case "turnstile":
        if (window.turnstile && window.turnstile.render) {
          widgetIdRef.current = window.turnstile.render(containerRef.current, {
            sitekey: siteKey,
            callback: onToken,
            "expired-callback": () => onToken(""),
          });
          readyRef.current = true;
        }
        break;
      default:
        break;
    }
  }, [provider, siteKey, onToken]);

  useEffect(() => {
    if (!provider || !siteKey) return;
    readyRef.current = false;

    // reCAPTCHA v3 — invisible, token on demand
    if (provider === "recaptcha_v3") {
      const src = SCRIPT_URLS.recaptcha_v3 + siteKey;
      loadScript(src).then(() => {
        if (window.grecaptcha) {
          window.grecaptcha.ready(() => {
            readyRef.current = true;
          });
        }
      });
      return;
    }

    // mCaptcha — uses an iframe-style vanilla widget from the instance
    if (provider === "mcaptcha") {
      if (!instanceUrl) return;
      const glueUrl = `${instanceUrl}/widget/?sitekey=${siteKey}`;
      loadScript(glueUrl).then(() => {
        readyRef.current = true;
      });
      // mCaptcha widget writes the token to an input with name="mcaptcha__token".
      // We poll for it since there's no callback API.
      const interval = setInterval(() => {
        const input = containerRef.current?.querySelector("input[name='mcaptcha__token']");
        if (input && input.value) {
          onToken(input.value);
        }
      }, 500);
      return () => clearInterval(interval);
    }

    // Explicit render for v2, hcaptcha, turnstile
    const callbackName =
      provider === "recaptcha_v2" ? "onRecaptchaLoad" :
      provider === "hcaptcha" ? "onHCaptchaLoad" :
      "onTurnstileLoad";

    window[callbackName] = renderWidget;

    loadScript(SCRIPT_URLS[provider]).then(() => {
      renderWidget();
    });

    return () => {
      delete window[callbackName];
    };
  }, [provider, siteKey, instanceUrl, renderWidget, onToken]);

  // reCAPTCHA v3 has no visible widget
  if (provider === "recaptcha_v3") return null;

  // mCaptcha renders into the container via its glue script
  if (provider === "mcaptcha") {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", my: 2 }}>
        <div ref={containerRef}>
          <div id="mcaptcha__widget-container"></div>
          <input type="hidden" name="mcaptcha__token" />
        </div>
      </Box>
    );
  }

  return (
    <Box sx={{ display: "flex", justifyContent: "center", my: 2 }}>
      <div ref={containerRef} />
    </Box>
  );
};

// Helper: for reCAPTCHA v3, call this before submit to get a fresh token.
export const executeRecaptchaV3 = (siteKey) => {
  return new Promise((resolve) => {
    if (window.grecaptcha) {
      window.grecaptcha.ready(() => {
        window.grecaptcha.execute(siteKey, { action: "submit" }).then(resolve);
      });
    } else {
      resolve("");
    }
  });
};

export default CaptchaWidget;
