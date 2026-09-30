// The Studio replica holding an edge's configuration stream. With replicas
// in more than one place (the Dashboard's Studio, headless control planes in
// MDCB), operators need to see which one serves an edge.

const DASH = '—';

// edgeOwnerText is the replica's label, or its node ID when it has none; a
// dash when no replica holds the edge.
export const edgeOwnerText = (edge) => {
  if (!edge?.ownerNodeId) return DASH;
  const name = edge.ownerLabel || edge.ownerNodeId;
  return edge.ownerLive === false ? `${name} (not responding)` : name;
};

// showHeldBy: the "Held by" column is worth its space only once the edges
// are spread over more than one replica.
export const showHeldBy = (edges) =>
  new Set((edges || []).map((e) => e.ownerNodeId).filter(Boolean)).size > 1;
