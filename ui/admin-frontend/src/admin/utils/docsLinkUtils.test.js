import { createDocsLinkHandler } from './docsLinkUtils';

describe('docsLinkUtils', () => {
  describe('createDocsLinkHandler', () => {
    // Mock window.open
    const originalWindowOpen = window.open;
    let windowOpenMock;

    beforeEach(() => {
      // Setup window.open mock
      windowOpenMock = jest.fn();
      window.open = windowOpenMock;
    });

    afterEach(() => {
      // Restore original window.open
      window.open = originalWindowOpen;
    });

    it('should create a handler that opens the documentation link in a new tab', () => {
      // Mock getDocsLink function
      const getDocsLink = jest.fn().mockReturnValue('https://docs.example.com/llm');
      
      // Create the handler
      const handler = createDocsLinkHandler(getDocsLink, 'llm_providers');
      
      // Execute the handler
      handler();
      
      // Verify getDocsLink was called with the correct key
      expect(getDocsLink).toHaveBeenCalledWith('llm_providers');
      
      // Verify window.open was called with the correct URL and target
      expect(windowOpenMock).toHaveBeenCalledWith('https://docs.example.com/llm', '_blank');
    });

    it('should not open a window if the link is not found', () => {
      // Mock getDocsLink function that returns null (link not found)
      const getDocsLink = jest.fn().mockReturnValue(null);
      
      // Create the handler
      const handler = createDocsLinkHandler(getDocsLink, 'invalid_key');
      
      // Execute the handler
      handler();
      
      // Verify getDocsLink was called with the correct key
      expect(getDocsLink).toHaveBeenCalledWith('invalid_key');
      
      // Verify window.open was not called
      expect(windowOpenMock).not.toHaveBeenCalled();
    });

    it('should not open a window if the link is an empty string', () => {
      // Mock getDocsLink function that returns an empty string
      const getDocsLink = jest.fn().mockReturnValue('');
      
      // Create the handler
      const handler = createDocsLinkHandler(getDocsLink, 'empty_link');
      
      // Execute the handler
      handler();
      
      // Verify getDocsLink was called with the correct key
      expect(getDocsLink).toHaveBeenCalledWith('empty_link');
      
      // Verify window.open was not called
      expect(windowOpenMock).not.toHaveBeenCalled();
    });
  });
});