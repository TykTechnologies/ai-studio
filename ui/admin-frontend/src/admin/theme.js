import { createTheme } from "@mui/material/styles";

const theme = createTheme({
  typography: {
    fontFamily: ["Inter-Regular", "sans-serif"].join(","),
    fontOpticalSizing: "auto",
    fontStyle: "normal",
    body1: {
      fontSize: "0.89rem",
    },
    body2: {
      fontSize: "0.89rem",
    },
  },
  palette: {
    mode: "light",
    primary: {
      main: "#23E2C2",
      light: "#82F5D8", // Light teal for hover
    },
    secondary: {
      main: "#21ecba",
    },
    background: {
      default: "#FFFFFF",
      defaultSubdued:'#E6E6EA', 
      neutralDefault: '#F0F0F3',
      secondaryExtraLight: '#f8f8f9',
      paper: "#FFFFFF",
      surfaceDefaultBoldest: '#031E3D',
      surfaceDefaultHover: '#B7F9E926',
      surfaceDefaultSelected: '#B7F9E973',
      buttonPrimaryDefault: '#343452',
      buttonPrimaryDefaultHover: '#181834',
      buttonPrimaryOutlineHover: '#EEFEFA',
      buttonSecondary: '#EDEDF0',
      buttonCritical: '#D82C0D',
      buttonCriticalHover: '#AE2410'
    },
    border: {
      neutralDefault: '#D8D8DF',
      neutralHovered: '#9D9DAF',
      criticalDefault: '#AE2410',
      criticalHover: '#8B1D12'
    },
    gray: {
      ligh: "#F5F5F5",
      main: "#CCCCCC",
      dark: "#333333",
    },
    custom: {
      white: "#FFFFFF",
      leaf: "#21ecba",
      purpleExtraDark: "#5900CB",
      purpleDark: "#8438FA",
      purpleLight: "#B421FA",
      purpleExtraLight: "#F0E4FF",
      teal: "#21ecba",
      lightTeal: "rgb(33 236 186 / 7%)",
      hoverTeal: "rgb(33 236 186 / 47%)",
      emptyStateBackground: "#23ebba11",
    },
    text: {
      primary: "#03031C",
      light: "#FFFFFF",
      dark: "#023056",
      default: "#03031C",
      defaultSubdued: '#414160'
    },
  },
  components: {
    MuiTypography: {
      styleOverrides: {
        root: ({ theme }) => ({
          color: theme.palette.text.primary 
        }),
        h5: {
          fontWeight: "bold",
        },
      },
    },
    MuiTableRow: {
      styleOverrides: {
        root: {
          "&:nth-of-type(odd)": {
            backgroundColor: "rgba(255, 255, 255, 1)",
          },
        },
      },
    },
    MuiTableCell: {
      styleOverrides: {
        root: {
          borderBottom: "1px solid rgba(255, 255, 255, 0.98)",
        },
      },
    },
    MuiPaper: {
      variants: [
        {
          props: { variant: "emptyState" },
          style: {
            backgroundColor: "#23ebba11", // Light blue color
            border: "1px solid #90CAF9", // Slightly darker blue for border
          },
        },
      ],
    },
    MuiInputLabel: {
      styleOverrides: {
        root: ({ theme }) => ({
          color: theme.palette.text.defaultSubdued,
          '&.Mui-focused': {
            color: theme.palette.text.defaultSubdued
          },
          '&.MuiInputLabel-shrink': {
            transform: "translate(14px, -9px) scale(0.75)"
          }
        })
      }
    },
    MuiOutlinedInput: {
      styleOverrides: {
        root: ({ theme }) => ({
          '& .MuiOutlinedInput-notchedOutline': {
            borderWidth: '1px',
            borderColor: theme.palette.border.neutralDefault
          },
          '&:hover .MuiOutlinedInput-notchedOutline': {
            borderWidth: '1px',
            borderColor: theme.palette.border.neutralDefault
          },
          '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
            borderWidth: '1px',
            borderColor: theme.palette.background.buttonPrimaryDefaultHover
          }
        })
      }
    },
    MuiFormLabel: {
      styleOverrides: {
        root: {
          "&.Mui-focused, &.MuiFormLabel-filled": {
            transform: "translate(2px, -16px) scale(0.75)", // Match the InputLabel
          },
        },
      },
    },
    MuiSelect: {
      styleOverrides: {
        root: ({ theme }) => ({
          '& .MuiOutlinedInput-notchedOutline': {
            borderColor: theme.palette.border.neutralDefault,
            '& legend': {
              height: '0px'
            }
          },
          '&.Mui-focused .MuiOutlinedInput-notchedOutline legend, & .Mui-focused .MuiOutlinedInput-notchedOutline legend': {
            height: '20px'
          },
          '&:hover .MuiOutlinedInput-notchedOutline': {
            borderColor: theme.palette.border.neutralDefault,
          },
          '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
            borderColor: theme.palette.background.buttonPrimaryDefaultHover,
          },
          '& .MuiSelect-icon': {
            color: theme.palette.text.default
          },
          '&.MuiSelect-pagination': {
            height: '32px',
            '& .MuiSelect-select': {
              height: '32px',
              padding: '8px 32px 8px 8px',
              display: 'flex',
              alignItems: 'center',
              lineHeight: '32px'
            },
            '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
              borderColor: theme.palette.border.neutralDefault,
              borderWidth: '1px'
            },
          }
        })
      }
    },
    MuiMenuItem: {
      styleOverrides: {
        root: ({ theme }) => ({
          '&.Mui-selected': {
            backgroundColor: theme.palette.background.secondaryExtraLight,
            fontFamily: 'Inter-Medium',
            '&:hover': {
              backgroundColor: theme.palette.background.secondaryExtraLight,
            }
          }
        })
      }
    },
    MuiButton: {
      styleOverrides: {
        root: ({ theme }) => ({
          position: 'relative',
          borderRadius: 20,
          padding: '8px 16px',
          color: theme.palette.text.defaultSubdued,
          backgroundColor: theme.palette.background.buttonSecondary,
          boxShadow: "none",
          textTransform: "capitalize",
          border: `1px solid ${theme.palette.border.neutralDefault}`,
          "&:hover": {
            backgroundColor: theme.palette.background.defaultSubdued,
            border: `1px solid ${theme.palette.border.neutralHovered}`,
            boxShadow: "none",
            color: theme.palette.text.defaultSubdued,
          },
        })
      }
    },
    MuiIconButton: {
      styleOverrides: {
        root: ({ theme }) => ({
          color: theme.palette.text.defaultSubdued,
          '&:hover': {
            backgroundColor: 'transparent',
          }
        })
      }
    },
    MuiCard: {
      styleOverrides: {
        root: {
          borderRadius: "16px",
          boxShadow: "0px 4px 20px rgba(0, 0, 0, 0.1)",
          backgroundColor: "#ffffff",
        },
      },
    },
  },
});

export default theme;
