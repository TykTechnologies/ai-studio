import { createTheme } from "@mui/material/styles";

const testTheme = createTheme({
  palette: {
    text: {
      primary: '#000000',
      secondary: '#666666',
      defaultSubdued: '#454545',
      neutralDisabled: '#AAAAAA'
    },
    background: {
      paper: '#FFFFFF',
      default: '#F5F5F5',
      surfaceNeutralHover: '#F0F0F0',
      surfaceNeutralDisabled: '#EEEEEE',
      buttonPrimaryDefault: '#3F51B5',
      buttonPrimaryDefaultHover: '#303F9F',
      buttonPrimaryOutlineHover: '#E8EAF6',
      defaultSubdued: '#FAFAFA'
    },
    border: {
      neutralDefault: '#E0E0E0',
      neutralHovered: '#BDBDBD',
      criticalDefault: '#F44336',
      criticalHover: '#D32F2F'
    },
    primary: {
      main: '#3F51B5',
      light: '#7986CB'
    },
    custom: {
      white: '#FFFFFF',
      purpleExtraDark: '#1A237E'
    }
  }
});

export default testTheme;