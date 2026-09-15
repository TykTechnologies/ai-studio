// Regenerates the JSON schema of the built-in generative UI ("present")
// client tool from the installed @assistant-ui/react-generative-ui library.
// The backend embeds the output (models/generative_ui_present_schema.json)
// so the model always sees the same component vocabulary the chat renders.
//
//   node scripts/gen-present-schema.mjs            # writes the file
//   node scripts/gen-present-schema.mjs --stdout   # prints instead
import { writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import { buildPresentParameters, defaultGenerativeUILibrary, JSONGenerativeUI } from '@assistant-ui/react-generative-ui';

const generative = new JSONGenerativeUI({ library: defaultGenerativeUILibrary });
const tool = generative.present();
const out = {
  description: tool.description,
  parameters: buildPresentParameters(defaultGenerativeUILibrary),
};

// The library's wire format keys nodes with `$type`, `$key` and `$action`,
// but Anthropic rejects property names starting with `$`
// ("Property keys should match pattern '^[a-zA-Z0-9_.-]{1,64}$'"). The model
// therefore sees `component`, `key` and `action`; the chat renderer maps them
// back (see chat-v2/parts/generativeTree.js). Keep both sides in step.
export const MODEL_KEYS = { $type: 'component', $key: 'key', $action: 'action' };
const renameKeys = (value) => {
  if (Array.isArray(value)) return value.map(renameKeys);
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.entries(value).map(([k, v]) => [MODEL_KEYS[k] || k, renameKeys(v)]));
  }
  if (typeof value === 'string') {
    return Object.entries(MODEL_KEYS).reduce((s, [from, to]) => s.split(`\`${from}\``).join(`\`${to}\``).split(from).join(to), value);
  }
  return value;
};

const json = `${JSON.stringify(renameKeys(out), null, 2)}\n`;

if (process.argv.includes('--stdout')) {
  process.stdout.write(json);
} else {
  const here = dirname(fileURLToPath(import.meta.url));
  const target = resolve(here, '../../../models/generative_ui_present_schema.json');
  writeFileSync(target, json);
  process.stdout.write(`wrote ${target} (${json.length} bytes)\n`);
}
