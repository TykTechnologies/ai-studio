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
    bodyXLargeDefault: {
      fontSize: "16px",
      lineHeight: "20px",
      fontFamily: "Inter-Regular",  
    },
    bodyLargeDefault: {
      fontSize: "14px",
      lineHeight: "20px",
      fontFamily: "Inter-Regular",
    },
    bodyLargeMedium: {
      fontSize: "14px",
      lineHeight: "20px",
      fontFamily: "Inter-Medium",
    },
    bodyLargeBold: {
      fontSize: "14px",
      lineHeight: "20px",
      fontFamily: "Inter-Bold",
    },
    bodyMediumDefault: {
      fontSize: "13.2px",
      lineHeight: "20px",
      fontFamily: "Inter-Regular",
    },
    bodyMediumSemiBold: {
      fontSize: "13.2px",
      lineHeight: "20px",
      fontFamily: "Inter-SemiBold",
    },
    bodySmallDefault: {
      fontSize: "12px",
      lineHeight: "16px",
      fontFamily: "Inter-Regular",
    },
    headingXLarge: {
      fontSize: "32px",
      lineHeight: "36px",
      fontFamily: "Inter-Bold",
    },
    headingXLargSub : {
      fontSize: "28px", 
      lineHeight: "36px",
      fontFamily: "Inter-SemiBold",
    },
    headingLarge: {
      fontSize: "20px",
      lineHeight: "24px",
      fontFamily: "Inter-Bold",
    },
    headingMedium: {
      fontSize: "18px",
      lineHeight: "24px",
      fontFamily: "Inter-Bold",
    },
    headingSmall: {
      fontSize: "14px",
      lineHeight: "20px", 
      fontFamily: "Inter-SemiBold", 
    }
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
      surfaceNeutralDisabled: '#FCFCFC',
      surfaceBrandDefaultPortal: '#EFF3FF',
      surfaceBrandDefaultDashboard: '#F6EFFF',
      surfaceBrandHovered: '#EBDFFE',
      surfaceDefaultBubble: '#B7F9E9C2',
      surfaceCriticalDefault: '#FCEFEC',
      surfaceWarningDefault: '#FFFBF2',
      surfaceSuccessDefault: '#EEF8F1',
      surfaceInformativeDefault: '#EBF8FE',
      iconSuccessDefault: '#2BA84A',
      iconWarningDefault: '#FFC453',
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
      neutralDefaultSubdued: '#F1F1F4',
      neutralPressed: '#656582',
      criticalDefault: '#AE2410',
      criticalDefaultSubdue: '#F9DDD8',
      criticalHover: '#8B1D12',
      successDefaultSubdued: '#DDF1E2 ',
      warningDefaultSubdued: '#FFF5E3',
      informativeDefaultSubdued: '#D6F1FC',
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
      purpleMedium: "#BB11FF",
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
      defaultSubdued: '#414160',
      linkDefault: "#00A6ED",
      criticalDefault: "#3E0E18",
      successDefault: "#0E3129",
      warningDefault: "#473717",
      neutralDisabled: "#818198",
    },
  },
  components: {

    MuiSvgIcon: {
      styleOverrides: {
        root: {
          width: '20px',
          height: '20px',
          fontSize: '20px',
        },
      },
    },
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
          textTransform: "none",
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
        root: ({ theme }) => ({
          borderRadius: "8px",
          border: `1px solid ${theme.palette.border.neutralDefault}`,
          backgroundColor: "#ffffff",
        })
      }
    },
  },
});

export default theme;
