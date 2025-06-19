const React = require('react');

const transferListStylesMock = {
  TransferListContainer: ({ children, ...props }) => 
    React.createElement('div', { 'data-testid': 'transfer-list-container', ...props }, children),
  TransferBox: ({ children, ...props }) => 
    React.createElement('div', { 'data-testid': 'transfer-box', ...props }, children),
  HeaderBox: ({ children, ...props }) => 
    React.createElement('div', { 'data-testid': 'header-box', ...props }, children),
  SearchBox: ({ children, ...props }) => 
    React.createElement('div', { 'data-testid': 'search-box', ...props }, children),
  SearchContainer: ({ children, ...props }) => 
    React.createElement('div', { 'data-testid': 'search-container', ...props }, children),
  AddButton: ({ onClick, ...props }) => 
    React.createElement('button', { 'data-testid': 'add-button-styled', onClick, ...props }),
  RemoveButton: ({ onClick, ...props }) => 
    React.createElement('button', { 'data-testid': 'remove-button-styled', onClick, ...props }),
};

const sharedStylesMock = {
  StyledTextField: ({ value, onChange, placeholder, error, helperText, inputProps, fullWidth, ...props }) => {
    const { children, ...restProps } = props;
    return React.createElement('input', { 
      'data-testid': 'styled-text-field', 
      'data-error': error?.toString(),
      'data-helper-text': helperText,
      'data-full-width': fullWidth?.toString(),
      'data-input-props': inputProps ? JSON.stringify(inputProps) : undefined,
      value, 
      onChange, 
      placeholder, 
      ...restProps 
    });
  },
  PrimaryButton: ({ children, onClick, disabled, type, ...props }) =>
    React.createElement('button', { 'data-testid': 'primary-button', onClick, disabled, type, ...props }, children),
  SecondaryOutlineButton: ({ children, onClick, ...props }) =>
    React.createElement('button', { 'data-testid': 'secondary-button', onClick, ...props }, children),
  TitleBox: ({ children, ...props }) =>
    React.createElement('div', { 'data-testid': 'title-box', ...props }, children),
  SecondaryLinkButton: ({ children, component, to, color, sx, startIcon, ...props }) => {
    const { onClick, ...domProps } = props;
    return React.createElement('a', { 'data-testid': 'secondary-link-button', href: to, onClick, ...domProps }, [
      React.createElement('span', { key: 'icon' }, startIcon),
      React.createElement('span', { key: 'children' }, children)
    ]);
  },
  TitleContentBox: ({ children, ...props }) =>
    React.createElement('div', { 'data-testid': 'title-content-box', ...props }, children),
  DangerOutlineButton: ({ children, onClick, ...props }) =>
    React.createElement('button', { 'data-testid': 'danger-outline-button', onClick, ...props }, children),
  StyledContentBox: ({ children, ...props }) =>
    React.createElement('div', { 'data-testid': 'styled-content-box', ...props }, children),
  StyledRadio: (props) => 
    React.createElement('input', { 'data-testid': 'styled-radio', type: 'radio', ...props }),
  StyledSwitch: ({ checked, onChange, ...props }) => 
    React.createElement('input', { 
      'data-testid': 'styled-switch', 
      type: 'checkbox',
      checked,
      onChange,
      onClick: () => onChange && onChange({ target: { checked: !checked } }),
      ...props 
    }),
  LearnMoreLink: ({ onClick, ...props }) =>
    React.createElement('a', { 'data-testid': 'learn-more-link', onClick, ...props }, 'Learn more'),
};

const actionModalStylesMock = {
  StyledActionDialog: ({ children, open, onClose, ...props }) => 
    React.createElement('div', { 'data-testid': 'action-dialog', 'data-open': open?.toString(), ...props }, children),
  TitleBox: ({ children, ...props }) => 
    React.createElement('div', { 'data-testid': 'title-box', ...props }, children),
  DialogDivider: (props) => 
    React.createElement('div', { 'data-testid': 'dialog-divider', ...props }),
  StyledDialogContent: ({ children, ...props }) => 
    React.createElement('div', { 'data-testid': 'styled-dialog-content', ...props }, children),
  StyledDialogActions: ({ children, ...props }) => 
    React.createElement('div', { 'data-testid': 'styled-dialog-actions', ...props }, children),
};

const chipStylesMock = {
  StyledChip: ({ 
    label, 
    size, 
    bgColor, 
    textColor, 
    onDelete, 
    onMouseDown, 
    deleteIcon, 
    ...props 
  }) => {
    const isSelectStyle = onDelete !== undefined;
    
    return React.createElement(
      'div', 
      { 
        'data-testid': isSelectStyle ? 'chip' : 'styled-chip', 
        'data-label': label,
        'data-size': size,
        'data-bg-color': bgColor,
        'data-text-color': textColor,
        ...props 
      }, 
      [
        label,
        isSelectStyle && onDelete && React.createElement(
          'button',
          { 
            'data-testid': 'chip-delete-button',
            'onClick': onDelete,
            'className': 'MuiChip-root',
            'key': 'delete-button'
          },
          deleteIcon
        )
      ].filter(Boolean)
    );
  }
};

const userFormStylesMock = {
  ButtonContainer: ({ children, ...props }) =>
    React.createElement('div', { 'data-testid': 'button-container', ...props }, children),
};

const userStylesMock = {
  RoleOptionBox: (props) => {
    const { children, value, control, label, isLast, ...otherProps } = props;
    return React.createElement('div', { 
      'data-testid': 'role-option-box',
      'data-value': value,
      'data-is-last': isLast?.toString(),
      ...otherProps 
    }, [
      React.createElement('div', { key: 'control' }, control),
      React.createElement('div', { key: 'label' }, label)
    ]);
  },
  RoleBadge: (props) => {
    const { children, bgColor, ...otherProps } = props;
    return React.createElement('div', { 
      'data-testid': 'role-badge',
      'data-bg-color': bgColor,
      ...otherProps 
    }, children);
  }
};

const authStylesMock = {
  StyledCheckbox: ({ checked, onChange, label }) => 
    React.createElement('div', {
      'data-testid': 'styled-checkbox',
      'data-checked': checked?.toString(),
      'data-label': label,
      onClick: () => onChange(!checked)
    })
};

module.exports = {
  transferListStylesMock,
  sharedStylesMock,
  actionModalStylesMock,
  chipStylesMock,
  userFormStylesMock,
  userStylesMock,
  authStylesMock
};