import React, { useEffect, useState } from 'react';
import { Alert, AlertTitle, Box, Link, Typography } from '@mui/material';
import apiClient from '../../../utils/apiClient';
import { listAll } from '../../../utils/listAll';
import { withBase } from '../../../../runtimeConfig';
import { CREDENTIAL_UNRESOLVED, CREDENTIAL_UNSET } from '../../llms/CredentialStatusIndicator';
import { usePermissions } from '../../../context/PermissionsContext';
import { P } from '../../../rbac/permissions';

/**
 * A fresh instance seeds active OpenAI and Anthropic providers whose keys
 * point at the empty OPENAI_KEY and ANTHROPIC_KEY secrets, so the first call
 * fails (#505). The provider list and detail page flag it; this repeats it
 * in the quick start, where a new user actually is, naming each secret to
 * fill in with a link to it.
 */

// Active providers whose credential cannot be used, as
// { id, name, secret } (secret is null for a provider with no key at all).
export const providersNeedingCredentials = (llms = []) =>
  llms
    .filter((llm) => llm?.attributes?.active)
    .filter((llm) =>
      [CREDENTIAL_UNRESOLVED, CREDENTIAL_UNSET].includes(llm.attributes.credential_status)
    )
    .map((llm) => ({
      id: llm.id,
      name: llm.attributes.name,
      secret: llm.attributes.credential_status === CREDENTIAL_UNRESOLVED ? llm.attributes.credential_ref || null : null,
    }));

// Maps each named secret to its id, from one bounded listing of the secrets.
// Names that cannot be found, or a caller without secrets:read, leave the
// link pointing at the secrets list.
const useSecretIds = (names) => {
  const [ids, setIds] = useState({});
  const key = names.join('\u0000');
  useEffect(() => {
    if (names.length === 0) return undefined;
    let cancelled = false;
    listAll(apiClient, '/secrets')
      .then((res) => {
        const wanted = new Set(names);
        const found = {};
        (res.data?.data || []).forEach((s) => {
          if (wanted.has(s.attributes?.var_name)) found[s.attributes.var_name] = s.id;
        });
        if (!cancelled) setIds(found);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  return ids;
};

// Links open in a new tab so the quick start keeps its place. Without an
// href (the user cannot open the target) the text stays, unlinked.
const NewTabLink = ({ href, children }) =>
  href ? (
    <Link href={withBase(href)} target="_blank" rel="noopener noreferrer">
      {children}
    </Link>
  ) : (
    <strong>{children}</strong>
  );

const CredentialSetupNotice = ({ llms, sx }) => {
  const broken = providersNeedingCredentials(llms);
  const secretNames = [...new Set(broken.map((p) => p.secret).filter(Boolean))];
  const { can } = usePermissions();
  const canEditSecrets = can(P.SECRETS_WRITE);
  // Only look secret ids up for someone who can open the edit page.
  const secretIds = useSecretIds(canEditSecrets ? secretNames : []);
  const secretHref = (name) => {
    if (canEditSecrets && secretIds[name]) return `/admin/secrets/edit/${secretIds[name]}`;
    return can(P.SECRETS_READ) ? '/admin/secrets' : null;
  };

  if (broken.length === 0) {
    return null;
  }

  return (
    <Alert severity="warning" sx={sx} data-testid="quickstart-credential-notice">
      <AlertTitle>Add your LLM provider keys first</AlertTitle>
      <Typography variant="bodyMediumDefault" component="p" sx={{ mb: 1 }}>
        {broken.length === 1 ? 'This provider is' : 'These providers are'} active but cannot make calls yet, so the first
        request through {broken.length === 1 ? 'it' : 'them'} will fail:
      </Typography>
      <Box component="ul" sx={{ m: 0, pl: 3 }}>
        {broken.map((p) => (
          <li key={p.id}>
            <Typography variant="bodyMediumDefault" component="span">
              {p.secret ? (
                <>
                  <strong>{p.name}</strong> uses the secret{' '}
                  <NewTabLink href={secretHref(p.secret)}>{p.secret}</NewTabLink>
                  , which has no value. Set it to your API key.
                </>
              ) : (
                <>
                  <strong>{p.name}</strong> has no API key.
                  {can(P.LLMS_WRITE) && (
                    <>
                      {' '}
                      <NewTabLink href={`/admin/llms/edit/${p.id}`}>Add one</NewTabLink>.
                    </>
                  )}
                </>
              )}
            </Typography>
          </li>
        ))}
      </Box>
    </Alert>
  );
};

export default CredentialSetupNotice;
