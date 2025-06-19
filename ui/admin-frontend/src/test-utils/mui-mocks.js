// This file exports mock objects that can be required directly in jest.mock() calls
// Usage: jest.mock('@mui/material', () => require('../test-utils/mui-mocks').muiMaterialMock);

const React = require('react');

// Mock components for direct use in tests
export const mockBoxComponent = ({ children, sx, ...props }) => React.createElement('div', { 'data-testid': 'box', 'data-sx': JSON.stringify(sx), ...props }, children);
export const mockTypographyComponent = ({ children, variant, color, sx, ...props }) => React.createElement('div', { 'data-testid': 'typography', 'data-variant': variant, 'data-color': color, 'data-sx': JSON.stringify(sx), ...props }, children);
export const mockDialogComponent = ({ children, open, ...props }) => React.createElement('div', { 'data-testid': 'dialog', 'data-open': open?.toString(), ...props }, children);
export const mockDialogContentComponent = ({ children, sx, ...props }) => React.createElement('div', { 'data-testid': 'dialog-content', 'data-sx': JSON.stringify(sx), ...props }, children);
export const mockDialogActionsComponent = ({ children, sx, ...props }) => React.createElement('div', { 'data-testid': 'dialog-actions', 'data-sx': JSON.stringify(sx), ...props }, children);
export const mockCircularProgressComponent = (props) => React.createElement('div', { 'data-testid': 'circular-progress', ...props });
export const mockMenuItemComponent = ({ children, value, selected, ...props }) => React.createElement('div', { 'data-testid': 'menu-item', 'data-value': value, 'data-selected': selected?.toString(), ...props }, children);
export const mockTextFieldComponent = ({ label, value, onChange, ...props }) => React.createElement('input', { 'data-testid': 'text-field', 'aria-label': label, value: value, onChange: onChange, ...props });
export const mockInputAdornmentComponent = ({ children, position, ...props }) => React.createElement('div', { 'data-testid': 'input-adornment', 'data-position': position, ...props }, children);

export const mockSearchIconComponent = (props) => React.createElement('div', { 'data-testid': 'search-icon', ...props });
export const mockAddIconComponent = (props) => React.createElement('div', { 'data-testid': 'add-icon', ...props });
export const mockCloseIconComponent = (props) => React.createElement('div', { 'data-testid': 'close-icon', ...props });

const muiMaterialMock = {
  __esModule: true,
  Box: ({ children, sx, alignItems, display, gap, ...props }) => {
    const filteredProps = { ...props };
    delete filteredProps.alignItems;
    delete filteredProps.display;
    delete filteredProps.gap;
    return React.createElement('div', { 
      'data-testid': 'box', 
      'data-sx': JSON.stringify(sx),
      'data-align-items': alignItems,
      'data-display': display,
      'data-gap': gap,
      ...filteredProps 
    }, children);
  },
  Typography: ({ children, variant, color, sx, ...props }) => React.createElement('div', { 'data-testid': 'typography', 'data-variant': variant, 'data-color': color, 'data-sx': JSON.stringify(sx), ...props }, children),
  Dialog: ({ children, open, ...props }) => React.createElement('div', { 'data-testid': 'dialog', 'data-open': open?.toString(), ...props }, children),
  DialogContent: ({ children, sx, ...props }) => React.createElement('div', { 'data-testid': 'dialog-content', 'data-sx': JSON.stringify(sx), ...props }, children),
  DialogActions: ({ children, sx, ...props }) => React.createElement('div', { 'data-testid': 'dialog-actions', 'data-sx': JSON.stringify(sx), ...props }, children),
  CircularProgress: (props) => React.createElement('div', { 'data-testid': 'circular-progress', ...props }),
  MenuItem: ({ children, value, selected, ...props }) => React.createElement('div', { 'data-testid': 'menu-item', 'data-value': value, 'data-selected': selected?.toString(), ...props }, children),
  TextField: ({ label, value, onChange, ...props }) => React.createElement('input', { 'data-testid': 'text-field', 'aria-label': label, value: value, onChange: onChange, ...props }),
  InputAdornment: ({ children, position, ...props }) => React.createElement('div', { 'data-testid': 'input-adornment', 'data-position': position, ...props }, children),
  Snackbar: ({ open, autoHideDuration, onClose, anchorOrigin, children }) => React.createElement('div', {
    'data-testid': 'snackbar',
    'data-open': open?.toString(),
    'data-duration': autoHideDuration,
    'data-vertical': anchorOrigin?.vertical,
    'data-horizontal': anchorOrigin?.horizontal
  }, [
    children,
    React.createElement('button', { 'data-testid': 'snackbar-close-button', onClick: onClose, key: 'close-button' })
  ]),
  Alert: ({ onClose, severity, sx, children }) => React.createElement('div', {
    'data-testid': 'alert',
    'data-severity': severity,
    'data-sx': JSON.stringify(sx)
  }, [
    children,
    React.createElement('button', { 'data-testid': 'alert-close-button', onClick: onClose, key: 'close-button' })
  ]),
  Paper: ({ children, ...props }) => React.createElement('div', { 'data-testid': 'paper', ...props }, children),
  IconButton: ({ children, onClick, ...props }) => React.createElement('button', { 'data-testid': 'icon-button', onClick, ...props }, children),
  Button: ({ children, onClick, ...props }) => React.createElement('button', { 'data-testid': 'button', onClick, ...props }, children),
  TableCell: ({ children, ...props }) => React.createElement('td', { 'data-testid': 'table-cell', ...props }, children),
  TableRow: ({ children, ...props }) => React.createElement('tr', { 'data-testid': 'table-row', ...props }, children),
  DialogTitle: ({ children, ...props }) => React.createElement('div', { 'data-testid': 'dialog-title', ...props }, children),
  Accordion: ({ children, ...props }) => React.createElement('div', { 'data-testid': 'accordion', ...props }, children),
  Select: ({ children, value, onChange, ...props }) => React.createElement('select', { 'data-testid': 'select', value, onChange, ...props }, children),
  FormControl: ({ children, ...props }) => React.createElement('div', { 'data-testid': 'form-control', ...props }, children),
  RadioGroup: ({ children, name, value, onChange, ...props }) => {
    return React.createElement('div', { 
      'data-testid': 'radio-group',
      'data-name': name,
      'data-value': value,
      'data-onchange': 'function',
      onClick: (e) => {
        if (onChange) {
          const targetValue = e.target?.value || e.target?.dataset?.value || value;
          onChange({ target: { value: targetValue } });
        }
      },
      ...props 
    }, children);
  },
  FormControlLabel: ({ value, control, label, ...props }) => React.createElement('div', { 
    'data-testid': 'form-control-label',
    'data-value': value,
    ...props 
  }, [control, label]),
};

const muiStyledEngineMock = {
  __esModule: true,
  default: () => () => ({}),
  styled: () => () => ({})
};

const muiStylesMock = {
  __esModule: true,
  styled: () => () => ({}),
  useTheme: () => require('./test-theme').default,
  createTheme: (theme) => theme,
  ThemeProvider: ({ children, theme, ...props }) => React.createElement('div', { 'data-testid': 'theme-provider', ...props }, children)
};

const muiSearchIconMock = {
  __esModule: true,
  default: (props) => React.createElement('div', { 'data-testid': 'search-icon', ...props })
};

const muiAddIconMock = {
  __esModule: true,
  default: (props) => React.createElement('div', { 'data-testid': 'add-icon', ...props })
};

const muiCloseIconMock = {
  __esModule: true,
  default: (props) => React.createElement('div', { 'data-testid': 'close-icon', ...props })
};

const muiIconButtonMock = {
  __esModule: true,
  default: ({ children, onClick, ...props }) => React.createElement('button', { 'data-testid': 'icon-button', onClick, ...props }, children)
};

module.exports = {
  muiMaterialMock,
  muiStyledEngineMock,
  muiStylesMock,
  muiSearchIconMock,
  muiAddIconMock,
  muiCloseIconMock,
  muiIconButtonMock
};