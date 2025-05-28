const React = require('react');

// Global variable to store the last TransferList props for testing
let lastTransferListProps = {};

const infiniteScrollContainerMock = {
  __esModule: true,
  default: ({ children, onLoadMore, hasMore, isLoading }) =>
    React.createElement('div', {
      'data-testid': "infinite-scroll-container",
      'data-has-more': hasMore,
      'data-is-loading': isLoading,
      onClick: onLoadMore
    }, children)
};

const transferListTableMock = {
  __esModule: true,
  default: ({ items, columns, idField, isLeftSide, onRemoveItem, onAddItem }) =>
    React.createElement('div', {
      'data-testid': isLeftSide ? "left-table" : "right-table",
      'data-items': JSON.stringify(items),
      'data-columns': JSON.stringify(columns),
      'data-id-field': idField
    }, isLeftSide ?
      React.createElement('button', { 'data-testid': "remove-button", onClick: () => items && items.length > 0 && onRemoveItem && onRemoveItem(items[0]) }, "Remove") :
      React.createElement('button', { 'data-testid': "add-button", onClick: () => items && items.length > 0 && onAddItem && onAddItem(items[0]) }, "Add")
    )
};

const transferListMock = {
  __esModule: true,
  default: props => {
    const React = require('react');
    lastTransferListProps = { ...props };
    
    return React.createElement('div', { 
      'data-testid': 'transfer-list'
    },
      React.createElement('div', { 
        'data-props': JSON.stringify({
          leftTitle: props.leftTitle,
          rightTitle: props.rightTitle,
          hasMore: props.hasMore,
          isLoadingMore: props.isLoadingMore,
          enableSearch: props.enableSearch
        })
      })
    );
  },
  // Export function to access last props for testing
  getLastProps: () => lastTransferListProps,
  // Export function to clear props for testing
  clearLastProps: () => { lastTransferListProps = {}; }
};

const collapsibleSectionMock = {
  __esModule: true,
  default: ({ children, title, defaultExpanded }) =>
    React.createElement('div', {
      'data-testid': "collapsible-section",
      'data-title': title,
      'data-default-expanded': defaultExpanded?.toString()
    }, children)
};

const customSelectBadgeMock = {
  __esModule: true,
  default: () => React.createElement('div', { 'data-testid': "custom-select-badge" })
};

module.exports = {
  infiniteScrollContainerMock,
  transferListTableMock,
  transferListMock,
  collapsibleSectionMock,
  customSelectBadgeMock
};
