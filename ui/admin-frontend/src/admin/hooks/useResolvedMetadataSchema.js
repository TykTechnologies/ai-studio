import { useEffect, useState } from 'react';
import { useEdition } from '../context/EditionContext';
import {
  resolveMetadataSchema,
  getMetadataVocabularies,
  getMetadataUsers,
} from '../services/governedMetadataService';

const EMPTY = { fields: [], vocabulariesBySlug: {}, users: [], usersById: {}, enforcement: 'advisory', loading: false };

/**
 * Loads the merged governed metadata schema for an object type, plus the
 * vocabularies and (when needed) users the inputs depend on.
 * Returns an empty schema in Community Edition or when nothing applies.
 */
const useResolvedMetadataSchema = (objectType) => {
  const { isEnterprise } = useEdition();
  const [state, setState] = useState({ ...EMPTY, loading: Boolean(isEnterprise && objectType) });

  useEffect(() => {
    let cancelled = false;
    if (!isEnterprise || !objectType) {
      setState({ ...EMPTY });
      return undefined;
    }
    setState((prev) => ({ ...prev, loading: true }));
    (async () => {
      try {
        const resolved = await resolveMetadataSchema(objectType);
        const fields = resolved?.fields || [];
        if (fields.length === 0) {
          if (!cancelled) setState({ ...EMPTY });
          return;
        }
        const needsVocab = fields.some((f) => f.type === 'vocabulary' || f.type === 'multi_vocabulary');
        const needsUsers = fields.some((f) => f.type === 'user');
        const [vocabularies, users] = await Promise.all([
          needsVocab ? getMetadataVocabularies() : Promise.resolve([]),
          needsUsers ? getMetadataUsers() : Promise.resolve([]),
        ]);
        const vocabulariesBySlug = {};
        vocabularies.forEach((v) => {
          vocabulariesBySlug[v.slug] = v;
        });
        const usersById = {};
        users.forEach((u) => {
          const attrs = u.attributes || u;
          usersById[String(u.id)] = attrs.name || attrs.email || `User ${u.id}`;
        });
        if (!cancelled) {
          setState({
            fields: [...fields].sort((a, b) => (a.order || 0) - (b.order || 0)),
            vocabulariesBySlug,
            users,
            usersById,
            enforcement: resolved.enforcement || 'advisory',
            loading: false,
          });
        }
      } catch (error) {
        if (!cancelled) setState({ ...EMPTY });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [isEnterprise, objectType]);

  return state;
};

export default useResolvedMetadataSchema;
