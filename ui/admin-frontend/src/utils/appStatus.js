// One status vocabulary for an App and its credential, shared by the admin
// list/detail pages and the portal (UX review M9). Every place that shows the
// state of an App derives it here so the words cannot drift again:
//
//   Active             the credential has been approved and the App is live
//   Awaiting approval  a credential exists but an administrator has not
//                      approved it yet
//   No credential      the App has no credential at all (admin only)
//   Disabled           an administrator switched the App off (is_active
//                      false), whatever its credential says
export const APP_STATUS = {
  ACTIVE: "Active",
  AWAITING_APPROVAL: "Awaiting approval",
  NO_CREDENTIAL: "No credential",
  DISABLED: "Disabled",
};

// Sort order for the admin list: live first, then the ones needing action,
// then the ones that cannot be used.
export const APP_STATUS_ORDER = {
  [APP_STATUS.ACTIVE]: 0,
  [APP_STATUS.AWAITING_APPROVAL]: 1,
  [APP_STATUS.NO_CREDENTIAL]: 2,
  [APP_STATUS.DISABLED]: 3,
};

/**
 * @param {object} args
 * @param {boolean|undefined} args.isActive App's is_active flag; undefined is
 *   treated as active because the portal's detail endpoint does not return it.
 * @param {boolean|undefined} args.credentialActive credential.active
 * @param {boolean} [args.hasCredential=true] false when the App has no
 *   credential at all (the admin list can tell; the portal always mints one).
 */
export const getAppStatus = ({ isActive, credentialActive, hasCredential = true }) => {
  if (isActive === false) {
    return APP_STATUS.DISABLED;
  }
  if (!hasCredential) {
    return APP_STATUS.NO_CREDENTIAL;
  }
  return credentialActive ? APP_STATUS.ACTIVE : APP_STATUS.AWAITING_APPROVAL;
};

/** Convenience for API objects: `{ attributes: { is_active, credential } }`. */
export const getAppStatusFromApp = (app) =>
  getAppStatus({
    isActive: app?.attributes?.is_active,
    credentialActive: app?.attributes?.credential?.active,
  });
