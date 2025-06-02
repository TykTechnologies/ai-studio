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
  StyledTextField: ({ value, onChange, placeholder, ...props }) => 
    React.createElement('input', { 'data-testid': 'styled-text-field', value, onChange, placeholder, ...props }),
  PrimaryButton: ({ children, onClick, ...props }) => 
    React.createElement('button', { 'data-testid': 'primary-button', onClick, ...props }, children),
  SecondaryOutlineButton: ({ children, onClick, ...props }) => 
    React.createElement('button', { 'data-testid': 'secondary-button', onClick, ...props }, children),
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

module.exports = {
  transferListStylesMock,
  sharedStylesMock,
  actionModalStylesMock
};