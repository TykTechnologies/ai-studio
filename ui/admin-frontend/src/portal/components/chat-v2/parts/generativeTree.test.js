import { toGenerativeTree } from './generativeTree';

describe('toGenerativeTree', () => {
  it('renames component/key/action on nodes and nests through children', () => {
    const tree = toGenerativeTree({
      component: 'Card',
      key: 'k1',
      title: 'T',
      confirm: { label: 'OK', action: { type: 'confirm', id: 7 } },
      children: [
        { component: 'Button', label: 'Go', action: { type: 'go', $input: undefined } },
        'plain text',
        { component: 'Row', children: [{ component: 'Fact', label: 'A', value: '1' }] },
      ],
    });
    expect(tree.$type).toBe('Card');
    expect(tree.$key).toBe('k1');
    expect(tree.component).toBeUndefined();
    expect(tree.confirm).toEqual({ label: 'OK', $action: { type: 'confirm', id: 7 } });
    expect(tree.children[0]).toMatchObject({ $type: 'Button', $action: { type: 'go' } });
    expect(tree.children[1]).toBe('plain text');
    expect(tree.children[2].children[0]).toEqual({ $type: 'Fact', label: 'A', value: '1' });
  });

  it('leaves prop values that merely contain those words alone', () => {
    const tree = toGenerativeTree({
      component: 'Table',
      columns: [{ label: 'component' }],
      rows: [['action', 'key']],
    });
    expect(tree).toEqual({ $type: 'Table', columns: [{ label: 'component' }], rows: [['action', 'key']] });
  });

  it('passes through non-node values', () => {
    expect(toGenerativeTree(null)).toBeNull();
    expect(toGenerativeTree('x')).toBe('x');
    expect(toGenerativeTree({ foo: 1 })).toEqual({ foo: 1 });
  });
});
