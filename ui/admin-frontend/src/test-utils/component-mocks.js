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

// Mock for the RelationshipPicker. Renders the selection as spans and exposes
// Add/Remove buttons that call `onChange` with the full new array, the way the
// real component does. Multiple pickers on one screen: use getAllByTestId.
let lastRelationshipPickerProps = {};

const relationshipPickerMock = {
  __esModule: true,
  default: (props) => {
    lastRelationshipPickerProps = { ...props };
    const idField = props.idField || 'id';
    const value = props.value || [];
    const options = props.options || [];
    const labelOf = (item) => (props.getOptionLabel ? props.getOptionLabel(item) : item.name);
    return React.createElement('div', {
      'data-testid': 'relationship-picker',
      'data-variant': props.variant || 'compact',
      'data-disabled': String(Boolean(props.disabled)),
      'data-item-label': props.itemLabel,
      'aria-label': props.label
    },
      React.createElement('span', { 'data-testid': 'relationship-picker-options-count' }, String(options.length)),
      value.map((item) => React.createElement('span', {
        key: String(item[idField]),
        'data-testid': 'relationship-picker-item',
        'data-item-id': item[idField]
      }, labelOf(item))),
      // type="button": the real picker has no submit control, and a bare
      // <button> inside a <form> would submit it (with stale state) on click.
      React.createElement('button', {
        type: 'button',
        'data-testid': 'relationship-picker-add',
        disabled: Boolean(props.disabled),
        onClick: () => props.onChange && options[0] && props.onChange([...value, options[0]])
      }, 'Add'),
      React.createElement('button', {
        type: 'button',
        'data-testid': 'relationship-picker-remove',
        disabled: Boolean(props.disabled),
        onClick: () => props.onChange && props.onChange(value.slice(1))
      }, 'Remove'),
      React.createElement('span', { 'data-testid': 'relationship-picker-caption' }, 'Changes apply when you save this form.')
    );
  },
  getLastProps: () => lastRelationshipPickerProps,
  clearLastProps: () => { lastRelationshipPickerProps = {}; }
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
  relationshipPickerMock,
  collapsibleSectionMock,
  customSelectBadgeMock
};
