import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import WarningBanner from '../WarningBanner';

// Mock dependencies
jest.mock('../Banner', () => {
  const Banner = ({
    title,
    message,
    onClose,
    linkText,
    linkUrl,
    showCloseButton,
    horizontalLayout,
    iconName,
    iconColor,
    borderColor,
    backgroundColor,
    titleColor,
    button,
    sx
  }) => (
    <div data-testid="mocked-banner">
      <div data-testid="banner-title">{title}</div>
      <div data-testid="banner-message">{message}</div>
      <div data-testid="banner-icon-name">{iconName}</div>
      <div data-testid="banner-icon-color">{iconColor}</div>
      <div data-testid="banner-border-color">{borderColor}</div>
      <div data-testid="banner-bg-color">{backgroundColor}</div>
      <div data-testid="banner-title-color">{titleColor}</div>
      {button && <div data-testid="banner-button">{button}</div>}
    </div>
  );
  return Banner;
});

jest.mock('../../../styles/sharedStyles', () => ({
  SecondaryOutlineButton: ({ children, onClick, size }) => (
    <button data-testid="secondary-outline-button" onClick={onClick}>
      {children}
    </button>
  )
}));

const renderComponent = (props = {}) => {
  return render(
    <WarningBanner
      title={props.title || "Warning Title"}
      message={props.message || "Warning Message"}
      onClose={props.onClose}
      linkText={props.linkText}
      linkUrl={props.linkUrl}
      showCloseButton={props.showCloseButton !== undefined ? props.showCloseButton : true}
      horizontalLayout={props.horizontalLayout}
      buttonText={props.buttonText}
      onButtonClick={props.onButtonClick}
      sx={props.sx}
    />
  );
};

describe('WarningBanner', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders correctly with basic props', () => {
    renderComponent();
    
    expect(screen.getByTestId('mocked-banner')).toBeInTheDocument();
    expect(screen.getByTestId('banner-title')).toHaveTextContent('Warning Title');
    expect(screen.getByTestId('banner-message')).toHaveTextContent('Warning Message');
  });

  it('passes correct icon and color props to Banner', () => {
    renderComponent();
    
    expect(screen.getByTestId('banner-icon-name')).toHaveTextContent('triangle-exclamation');
    expect(screen.getByTestId('banner-border-color')).toHaveTextContent('border.warningDefaultSubdued');
    expect(screen.getByTestId('banner-bg-color')).toHaveTextContent('background.surfaceWarningDefault');
    expect(screen.getByTestId('banner-title-color')).toHaveTextContent('text.warningDefault');
  });

  it('renders with custom title and message', () => {
    renderComponent({
      title: 'Custom Warning',
      message: 'Custom Message'
    });
    
    expect(screen.getByTestId('banner-title')).toHaveTextContent('Custom Warning');
    expect(screen.getByTestId('banner-message')).toHaveTextContent('Custom Message');
  });

  it('renders with button when buttonText and onButtonClick are provided', () => {
    const onButtonClick = jest.fn();
    
    renderComponent({
      buttonText: 'Take Action',
      onButtonClick
    });
    
    expect(screen.getByTestId('banner-button')).toBeInTheDocument();
    expect(screen.getByTestId('secondary-outline-button')).toHaveTextContent('Take Action');
  });

  it('does not render button when buttonText is not provided', () => {
    const onButtonClick = jest.fn();
    
    renderComponent({
      onButtonClick
    });
    
    expect(screen.queryByTestId('banner-button')).not.toBeInTheDocument();
  });

  it('does not render button when onButtonClick is not provided', () => {
    renderComponent({
      buttonText: 'Take Action'
    });
    
    expect(screen.queryByTestId('banner-button')).not.toBeInTheDocument();
  });

  it('button triggers onButtonClick when clicked', () => {
    const onButtonClick = jest.fn();
    
    renderComponent({
      buttonText: 'Take Action',
      onButtonClick
    });
    
    const button = screen.getByTestId('secondary-outline-button');
    fireEvent.click(button);
    
    expect(onButtonClick).toHaveBeenCalledTimes(1);
  });
});