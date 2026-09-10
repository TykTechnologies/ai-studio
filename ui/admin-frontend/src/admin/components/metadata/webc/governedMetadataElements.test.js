import { act } from 'react';

// These elements are mounted by hand (no @testing-library render), so tell React
// that act() is the expected way to drive updates here.
globalThis.IS_REACT_ACT_ENVIRONMENT = true;
import registerGovernedMetadataElements, {
  GOVERNED_METADATA_FIELDS_TAG,
  GOVERNED_METADATA_BADGES_TAG,
} from './governedMetadataElements';
import {
  resolveMetadataSchema,
  getMetadataVocabularies,
  getMetadataUsers,
  validateObjectMetadata,
} from '../../../services/governedMetadataService';

jest.mock('../../../services/governedMetadataService', () => {
  const actual = jest.requireActual('../../../services/governedMetadataService');
  return {
    ...actual,
    resolveMetadataSchema: jest.fn(),
    getMetadataVocabularies: jest.fn(),
    getMetadataUsers: jest.fn(),
    validateObjectMetadata: jest.fn(),
  };
});

const FIELDS = [
  { key: 'risk_tier', label: 'Risk tier', type: 'vocabulary', vocabulary_slug: 'risk_tier', required: true, order: 2 },
  { key: 'owner_note', label: 'Owner note', type: 'string', order: 1 },
];
const TERMS = [
  { value: 'low', label: 'Low' },
  { value: 'high', label: 'High' },
];

const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });

const mount = async (tag, setup = () => {}) => {
  const el = document.createElement(tag);
  setup(el);
  await act(async () => {
    document.body.appendChild(el);
  });
  await flush();
  return el;
};

const shadowText = (el) => el.shadowRoot.textContent;
const shadowQuery = (el, selector) => el.shadowRoot.querySelector(selector);

describe('governed metadata host Web Components', () => {
  beforeAll(() => {
    registerGovernedMetadataElements();
  });

  afterEach(async () => {
    // Detach inside act and let the deferred unmounts settle before jsdom is torn down.
    await act(async () => {
      document.body.innerHTML = '';
    });
    await flush();
  });

  beforeEach(() => {
    resolveMetadataSchema.mockResolvedValue({ fields: FIELDS, enforcement: 'enforce' });
    getMetadataVocabularies.mockResolvedValue([{ slug: 'risk_tier', terms: TERMS }]);
    getMetadataUsers.mockResolvedValue([]);
    validateObjectMetadata.mockResolvedValue({ valid: true, enforced: true, errors: [], warnings: [] });
  });

  it('registers both elements exactly once', () => {
    expect(customElements.get(GOVERNED_METADATA_FIELDS_TAG)).toBeDefined();
    expect(customElements.get(GOVERNED_METADATA_BADGES_TAG)).toBeDefined();
    expect(() => registerGovernedMetadataElements()).not.toThrow();
  });

  it('loads the schema from the host API, renders fields in order inside its shadow root and emits ready', async () => {
    const ready = jest.fn();
    const el = await mount(GOVERNED_METADATA_FIELDS_TAG, (node) => {
      node.setAttribute('object-type', 'plugin_resource:12:prompts');
      node.addEventListener('ready', ready);
    });

    expect(resolveMetadataSchema).toHaveBeenCalledWith('plugin_resource:12:prompts');
    expect(getMetadataVocabularies).toHaveBeenCalled();
    expect(ready).toHaveBeenCalledTimes(1);
    expect(ready.mock.calls[0][0].detail.enforcement).toBe('enforce');
    expect(ready.mock.calls[0][0].detail.fields.map((f) => f.key)).toEqual(['owner_note', 'risk_tier']);

    const text = shadowText(el);
    expect(text).toContain('Governance Metadata');
    expect(text).toContain('Enforced');
    expect(text).toContain('Owner note');
    expect(text).toContain('Risk tier');
    // Styles are scoped to the shadow root, not leaked into the document head.
    expect(el.shadowRoot.querySelector('style')).not.toBeNull();
    expect(document.body.textContent).not.toContain('Governance Metadata');
  });

  it('emits change with normalised values, exposes getValues() and validates live through the host', async () => {
    const changes = [];
    const el = await mount(GOVERNED_METADATA_FIELDS_TAG, (node) => {
      node.objectType = 'tool';
      node.value = { owner_note: 'keep me' };
      node.addEventListener('change', (e) => changes.push(e.detail));
    });

    const input = shadowQuery(el, '#governed-metadata-field-owner_note');
    expect(input.value).toBe('keep me');
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
      setter.call(input, 'new note');
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    expect(changes).toHaveLength(1);
    expect(changes[0]).toEqual({ values: { owner_note: 'new note' }, objectType: 'tool' });
    expect(el.getValues()).toEqual({ owner_note: 'new note' });

    // Clearing a field drops the key instead of sending an empty string.
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
      setter.call(input, '');
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    expect(changes[1].values).toEqual({});

    // Live validation runs (debounced) against the host API when the schema came from the host.
    validateObjectMetadata.mockResolvedValue({
      valid: false, enforced: true,
      errors: [{ field: 'risk_tier', code: 'required', message: 'Risk tier is required' }], warnings: [],
    });
    await act(async () => {
      await new Promise((r) => setTimeout(r, 450));
    });
    expect(validateObjectMetadata).toHaveBeenCalledWith('tool', {});
    expect(shadowText(el)).toContain('Risk tier is required');
  });

  it('validate() returns the result and maps field errors; setErrors shows a form-level message', async () => {
    const el = await mount(GOVERNED_METADATA_FIELDS_TAG, (node) => {
      node.objectType = 'llm';
    });
    validateObjectMetadata.mockResolvedValue({
      valid: false, enforced: true,
      errors: [{ field: 'risk_tier', code: 'required', message: 'Risk tier is required' }], warnings: [],
    });
    let result;
    await act(async () => {
      result = await el.validate();
    });
    expect(result.valid).toBe(false);
    expect(el.errors).toEqual({ risk_tier: 'Risk tier is required' });
    await flush();
    expect(shadowText(el)).toContain('Risk tier is required');

    await act(async () => {
      el.setErrors({ _: 'Governance metadata failed validation', owner_note: 'Too long' });
    });
    await flush();
    expect(shadowText(el)).toContain('Governance metadata failed validation');
    expect(shadowText(el)).toContain('Too long');

    const bare = await mount(GOVERNED_METADATA_FIELDS_TAG);
    await expect(bare.validate()).rejects.toThrow(/object-type is required/);
  });

  it('renders from a plugin-provided schema without touching the host API', async () => {
    const el = await mount(GOVERNED_METADATA_FIELDS_TAG, (node) => {
      node.schema = { fields: FIELDS, vocabularies: { risk_tier: TERMS }, enforcement: 'advisory' };
      node.value = { risk_tier: 'high' };
    });
    expect(resolveMetadataSchema).not.toHaveBeenCalled();
    expect(shadowText(el)).toContain('Advisory');
    expect(shadowText(el)).toContain('High');

    // The same payload as a JSON attribute works for non-JS integrations.
    const viaAttr = await mount(GOVERNED_METADATA_FIELDS_TAG, (node) => {
      node.setAttribute('schema', JSON.stringify({ fields: FIELDS, vocabularies: { risk_tier: TERMS } }));
      node.setAttribute('value', JSON.stringify({ owner_note: 'from attribute' }));
    });
    expect(shadowQuery(viaAttr, '#governed-metadata-field-owner_note').value).toBe('from attribute');
    // Garbage JSON is ignored rather than crashing the host page.
    const broken = await mount(GOVERNED_METADATA_FIELDS_TAG, (node) => {
      node.setAttribute('schema', '{not json');
      node.setAttribute('value', '{nope');
    });
    expect(broken.shadowRoot).not.toBeNull();
    expect(broken.getValues()).toEqual({});
  });

  it('renders nothing when no fields apply and a warning when the host API refuses', async () => {
    resolveMetadataSchema.mockResolvedValue({ fields: [], enforcement: 'advisory' });
    const none = await mount(GOVERNED_METADATA_FIELDS_TAG, (node) => {
      node.objectType = 'datasource';
    });
    expect(shadowText(none)).not.toContain('Governance Metadata');

    resolveMetadataSchema.mockRejectedValue({ response: { status: 403 } });
    const errored = jest.fn();
    const refused = await mount(GOVERNED_METADATA_FIELDS_TAG, (node) => {
      node.objectType = 'datasource';
      node.addEventListener('error', errored);
    });
    expect(errored).toHaveBeenCalledTimes(1);
    expect(shadowText(refused)).toContain('Enterprise feature');
  });

  it('unmounts cleanly when removed and re-renders when re-attached', async () => {
    const el = await mount(GOVERNED_METADATA_FIELDS_TAG, (node) => {
      node.objectType = 'tool';
    });
    expect(shadowText(el)).toContain('Governance Metadata');
    await act(async () => {
      el.remove();
    });
    await flush();
    await act(async () => {
      document.body.appendChild(el);
    });
    await flush();
    expect(shadowText(el)).toContain('Governance Metadata');
  });

  it('badges element renders the portal display list from a property or attribute', async () => {
    const items = [
      { key: 'data_classification', label: 'Data classification', type: 'vocabulary', value: 'Confidential' },
      { key: 'regs', label: 'Regulations', type: 'multi_vocabulary', value: ['GDPR', 'HIPAA'] },
    ];
    const viaProp = await mount(GOVERNED_METADATA_BADGES_TAG, (node) => {
      node.items = items;
    });
    expect(shadowText(viaProp)).toContain('Data classification: Confidential');
    expect(shadowText(viaProp)).toContain('Regulations: GDPR, HIPAA');

    const viaAttr = await mount(GOVERNED_METADATA_BADGES_TAG, (node) => {
      node.setAttribute('items', JSON.stringify(items.slice(0, 1)));
    });
    expect(shadowText(viaAttr)).toContain('Data classification: Confidential');
    expect(shadowText(viaAttr)).not.toContain('Regulations');

    const empty = await mount(GOVERNED_METADATA_BADGES_TAG);
    expect(shadowText(empty).trim()).toBe('');
    await act(async () => {
      empty.items = '[broken';
    });
    expect(shadowText(empty).trim()).toBe('');
  });
});
