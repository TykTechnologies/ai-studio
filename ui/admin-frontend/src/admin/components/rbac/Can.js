import { usePermissions } from '../../context/PermissionsContext';
import { toArray } from '../../rbac/permissions';

/**
 * Renders children only when the user holds the permission(s).
 *
 *   <Can permission={P.LLMS_WRITE}>...</Can>
 *   <Can anyOf={[P.USERS_WRITE, P.GROUPS_WRITE]}>...</Can>
 *   <Can allOf={[...]} fallback={<Locked />}>...</Can>
 *   <Can permission={P.X}>{(allowed) => ...}</Can>   // render-prop form
 *   <Can permission={P.LLMS_WRITE} workspaceId={llm.workspace_id}>...</Can>  // in the object's Workspace
 *   <Can permission={P.LLMS_WRITE} anywhere>...</Can>  // globally or in any Workspace (create)
 */
const Can = ({ permission, anyOf, allOf, workspaceId, anywhere = false, fallback = null, children }) => {
  const { can, canIn, canAnywhere, canAny, canAll } = usePermissions();

  let allowed = true;
  if (permission) {
    // canIn/canAnywhere may be absent where tests mock usePermissions.
    if (workspaceId !== undefined) allowed = allowed && (canIn ? canIn(workspaceId, permission) : can(permission));
    else if (anywhere) allowed = allowed && (canAnywhere ? canAnywhere(permission) : can(permission));
    else allowed = allowed && can(permission);
  }
  if (anyOf) allowed = allowed && canAny(toArray(anyOf));
  if (allOf) allowed = allowed && canAll(toArray(allOf));

  if (typeof children === 'function') {
    return children(allowed);
  }
  return allowed ? children : fallback;
};

export default Can;
