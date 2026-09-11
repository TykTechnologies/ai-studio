import { usePermissions } from '../../context/PermissionsContext';
import { toArray } from '../../rbac/permissions';

/**
 * Renders children only when the user holds the permission(s).
 *
 *   <Can permission={P.LLMS_WRITE}>...</Can>
 *   <Can anyOf={[P.USERS_WRITE, P.GROUPS_WRITE]}>...</Can>
 *   <Can allOf={[...]} fallback={<Locked />}>...</Can>
 *   <Can permission={P.X}>{(allowed) => ...}</Can>   // render-prop form
 */
const Can = ({ permission, anyOf, allOf, fallback = null, children }) => {
  const { can, canAny, canAll } = usePermissions();

  let allowed = true;
  if (permission) allowed = allowed && can(permission);
  if (anyOf) allowed = allowed && canAny(toArray(anyOf));
  if (allOf) allowed = allowed && canAll(toArray(allOf));

  if (typeof children === 'function') {
    return children(allowed);
  }
  return allowed ? children : fallback;
};

export default Can;
