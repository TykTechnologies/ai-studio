import React from 'react';
import { render } from '@testing-library/react';
import { ThemeProvider } from '@mui/material/styles';
import { createTheme } from '@mui/material';

// Create a default theme for testing
export const testTheme = createTheme({
  palette: {
    background: {
      paper: '#ffffff',
      default: '#f5f5f5',
      buttonPrimaryDefault: '#007bff',
      buttonPrimaryDefaultHover: '#0056b3',
      buttonPrimaryOutlineHover: '#e6f0ff',
      surfaceNeutralHover: '#f0f0f0',
      surfaceNeutralDisabled: '#f5f5f5',
      defaultSubdued: '#cccccc',
      buttonCritical: '#dc3545',
      buttonCriticalHover: '#c82333',
    },
    text: {
      primary: '#000000',
      secondary: '#666666',
      default: '#212121',
      defaultSubdued: '#757575',
      neutralDisabled: '#9e9e9e',
    },
    primary: {
      main: '#007bff',
      light: '#4dabf5',
      dark: '#0056b3',
    },
    border: {
      neutralDefault: '#dddddd',
      neutralHovered: '#bbbbbb',
      criticalDefault: '#dc3545',
      criticalHover: '#c82333',
      criticalDefaultSubdue: '#f8d7da',
    },
    custom: {
      white: '#ffffff',
      teal: '#20c997',
      lightTeal: '#e6f8f5',
      purpleExtraDark: '#6610f2',
    },
  },
  spacing: (factor) => `${8 * factor}px`,
  shape: {
    borderRadius: 4,
  },
});

// Render component wrapped with ThemeProvider
export const renderWithTheme = (ui, options = {}) => {
  return render(
    <ThemeProvider theme={testTheme}>
      {ui}
    </ThemeProvider>,
    options
  );
};

// Mock Material UI components at the module level
// This must be outside of any function as Jest hoists mocks
jest.mock('@mui/material', () => {
  const originalModule = jest.requireActual('@mui/material');
  
  // Create simple factory functions that don't use JSX
  const createElementMock = (type, props, ...children) => {
    return { type, props: { ...props, children: children.length ? children : undefined } };
  };

  return {
    ...originalModule,
    MenuItem: function MockMenuItem(props) {
      return createElementMock('option', { 'data-testid': 'menu-item', ...props });
    },
    Typography: function MockTypography(props) {
      return createElementMock('span', { 'data-testid': `typography-${props.variant || 'default'}`, ...props });
    },
    Box: function MockBox(props) {
      return createElementMock('div', { 'data-testid': 'mui-box', ...props });
    },
    Chip: function MockChip(props) {
      const chipProps = { 'data-testid': 'mui-chip', ...props };
      const chipElement = createElementMock('div', chipProps);
      
      if (props.onDelete) {
        const deleteButton = createElementMock('button', {
          'data-testid': 'chip-delete-button',
          onClick: props.onDelete
        }, '×');
        
        if (!chipElement.props.children) {
          chipElement.props.children = [];
        }
        
        chipElement.props.children = [props.label, deleteButton];
      }
      
      return chipElement;
    },
    InputLabel: function MockInputLabel(props) {
      return createElementMock('label', { 'data-testid': 'input-label', ...props });
    },
    FormControl: function MockFormControl(props) {
      return createElementMock('div', { 'data-testid': 'form-control', ...props });
    },
    Select: function MockSelect(props) {
      const selectElement = createElementMock('select', {
        'data-testid': 'select-element',
        onChange: props.onChange ?
          (e) => props.onChange({ target: { value: e.target.value } }) : undefined,
        value: props.value,
        multiple: props.multiple
      }, props.children);
      
      const mockSelectWrapper = createElementMock('div', { 'data-testid': 'mui-select', ...props }, selectElement);
      
      if (props.renderValue && props.value) {
        const renderValueElement = createElementMock('div', { 'data-testid': 'render-value' }, props.renderValue(props.value));
        mockSelectWrapper.props.children = [selectElement, renderValueElement];
      }
      
      return mockSelectWrapper;
    }
  };
});