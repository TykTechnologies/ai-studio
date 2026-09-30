import { edgeOwnerText, showHeldBy } from './edgeOwner';

describe('edgeOwnerText', () => {
  it('names the replica by its label', () => {
    expect(edgeOwnerText({ ownerNodeId: 'host-1-abc', ownerLabel: 'mdcb-eu-1', ownerLive: true }))
      .toBe('mdcb-eu-1');
  });

  it('falls back to the node ID when the replica has no label', () => {
    expect(edgeOwnerText({ ownerNodeId: 'host-1-abc', ownerLabel: '', ownerLive: true }))
      .toBe('host-1-abc');
  });

  it('shows a dash when no replica holds the edge', () => {
    expect(edgeOwnerText({ ownerNodeId: '', ownerLabel: '' })).toBe('—');
    expect(edgeOwnerText({})).toBe('—');
  });

  it('marks a replica that is no longer live', () => {
    expect(edgeOwnerText({ ownerNodeId: 'n', ownerLabel: 'dashboard', ownerLive: false }))
      .toBe('dashboard (not responding)');
  });
});

describe('showHeldBy', () => {
  it('is hidden with one replica or none', () => {
    expect(showHeldBy([])).toBe(false);
    expect(showHeldBy([{ ownerNodeId: 'a' }, { ownerNodeId: 'a' }, { ownerNodeId: '' }])).toBe(false);
  });

  it('shows once the edges are held by more than one replica', () => {
    expect(showHeldBy([{ ownerNodeId: 'a' }, { ownerNodeId: 'b' }])).toBe(true);
  });
});
