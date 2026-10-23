import {
  PLUGIN_SOURCES,
  pluginSourceOf,
  allowedPluginSources,
  pluginSourceError,
  pluginCommandHelp,
  pluginCommandPlaceholder,
} from './pluginService';

jest.mock('../utils/apiClient');

describe('pluginSourceOf', () => {
  // The same table as the backend's plugin_security.SourceOf tests.
  test.each([
    ['oci://registry.example.com/plugins/demo:1.0.0', 'oci'],
    ['grpc://plugin.example.com:50051', 'grpc'],
    ['file:///opt/plugins/demo', 'file'],
    ['/opt/plugins/demo', 'file'],
    ['plugins/demo', 'file'],
    ['./demo --flag', 'file'],
    ['http://example.com/plugin', 'file'],
    ['https://example.com/plugin', 'file'],
    ['', 'file'],
    ['OCI://registry.example.com/x', 'file'],
    [' oci://registry.example.com/x', 'file'],
    ['\toci://registry.example.com/x', 'file'],
    ['Grpc://plugin.example.com:1', 'file'],
    ['oci:/registry.example.com/x', 'file'],
  ])('%j is %s', (command, source) => {
    expect(pluginSourceOf(command)).toBe(source);
  });
});

describe('allowedPluginSources', () => {
  test('all sources when the config does not say', () => {
    expect(allowedPluginSources(null)).toEqual(PLUGIN_SOURCES);
    expect(allowedPluginSources({})).toEqual(PLUGIN_SOURCES);
    expect(allowedPluginSources({ plugin_allowed_sources: [] })).toEqual(PLUGIN_SOURCES);
  });

  test('the configured list', () => {
    expect(allowedPluginSources({ plugin_allowed_sources: ['oci'] })).toEqual(['oci']);
  });
});

describe('pluginSourceError', () => {
  test('null for an allowed source', () => {
    expect(pluginSourceError('oci://r/x:1', ['oci'])).toBeNull();
  });

  test("the backend's wording for a disallowed source", () => {
    expect(pluginSourceError('/opt/plugins/demo', ['oci'])).toBe(
      '🔒 SECURITY: plugin source "file" is not allowed by this installation (allowed: oci). Set AI_STUDIO_PLUGIN_ALLOWED_SOURCES to change this.'
    );
    expect(pluginSourceError('grpc://h:1', ['oci', 'file'])).toContain('(allowed: oci, file)');
  });
});

describe('command help and placeholder', () => {
  test('unchanged with every source allowed', () => {
    expect(pluginCommandHelp(PLUGIN_SOURCES)).toBe(
      'Plugin command - use oci:// for OCI artifacts, grpc:// for external services, or local path for binaries'
    );
    expect(pluginCommandPlaceholder(PLUGIN_SOURCES)).toBe(
      'e.g., oci://registry.com/my-plugin:latest or /path/to/plugin-binary'
    );
  });

  test('only the allowed sources, with the signing note', () => {
    const help = pluginCommandHelp(['oci'], true);
    expect(help).toBe('Plugin command - use oci:// for OCI artifacts (they must be signed)');
    expect(help).not.toContain('grpc://');
    expect(help).not.toContain('local path');
    expect(pluginCommandPlaceholder(['oci'])).toBe('e.g., oci://registry.com/my-plugin:latest');
    expect(pluginCommandHelp(['file'])).toBe('Plugin command - use local path for binaries');
  });
});
