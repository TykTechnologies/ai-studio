import { styled } from '@mui/material/styles';
import { 
  Dialog, 
  DialogTitle, 
  DialogContent, 
  Box, 
  Step, 
  StepLabel, 
  Stepper, 
} from '@mui/material';

export const StyledDialog = styled(Dialog)(({ theme }) => ({
  zIndex: 1500, 
  '& .MuiDialog-paper': {
    borderRadius: 16,
    border: `1px solid ${theme.palette.border.neutralPressed}`,
    boxShadow: '0px 16px 32px 0px rgba(9, 9, 35, 0.25)',
    maxWidth: '80%',
    maxHeight:'90%',
    margin: '0 auto',
  },
}));

export const StyledDialogTitle = styled(DialogTitle)(({ theme }) => ({
  padding: theme.spacing(3),
  backgroundColor: theme.palette.background.default,
  color: theme.palette.text.default,
}));

export const StyledDialogContent = styled(DialogContent)(({ theme }) => ({
  padding: theme.spacing(5),
}));

export const StyledStepper = styled(Stepper)(({ theme }) => ({
  marginBottom: theme.spacing(4),
  '& .MuiStepConnector-line': {
    display: 'none',
  },
}));

export const StyledStep = styled(Step)(({ theme, active }) => ({
  '& .MuiStepLabel-root': {
    flexDirection: 'row',
    alignItems: 'center',
    padding: theme.spacing(1, 0),
    borderBottom: `1px solid ${active 
      ? theme.palette.background.iconSuccessDefault 
      : theme.palette.border.neutralDefaultSubdued}`,
  },
}));

export const StyledStepLabel = styled(StepLabel)(({ theme, active }) => ({
  '& .MuiStepLabel-iconContainer': {
    width: 24,
    height: 24,
    borderRadius: '50%',
    backgroundColor: 'transparent',
    border: `1px solid ${active 
      ? theme.palette.background.iconSuccessDefault 
      : theme.palette.border.neutralDefault}`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    padding: 0,
    marginRight: theme.spacing(2),
    position: 'relative',
    '& .MuiStepIcon-root': {
      display: 'none',
    },
    '&::before': {
      content: 'attr(data-step-number)',
      position: 'absolute',
      fontSize: '14px',
      fontWeight: 500,
      color: active 
        ? theme.palette.text.primary 
        : theme.palette.text.neutralDisabled,
    },
  },
  '& .MuiStepLabel-label': {
    ...(active 
      ? theme.typography.bodyLargeBold 
      : theme.typography.bodyLargeDefault),
    color: active 
      ? theme.palette.text.primary 
      : theme.palette.text.neutralDisabled,
    textAlign: 'left',
  },
}));

export const ActionsContainer = styled(Box)(({ theme }) => ({
  display: 'flex',
  justifyContent: 'center',
  marginTop: theme.spacing(1),
  gap: theme.spacing(2),
  '@media (max-width: 600px)': {
    flexDirection: 'column',
    width: '100%',
    alignItems: 'center',
    '& > button': {
      width: '100%',
      marginBottom: '0.5rem',
    },
  },
}));

export const LeftActions = styled(Box)({
  display: 'flex',
});

export const RightActions = styled(Box)({
  display: 'flex',
  gap: 16,
});

export const StepperContainer = styled(Box)(({ theme }) => ({
  display: 'flex',
  justifyContent: 'flex-start',
  alignItems: 'center',
  width: '100%',
  marginBottom: theme.spacing(2),
  position: 'relative',
  paddingBottom: theme.spacing(1),
}));

export const StepConnector = styled(Box)(({ theme }) => ({
  position: 'absolute',
  bottom: 0,
  left: 0,
  right: 0,
  height: '1.6px',
  backgroundColor: theme.palette.border.neutralDefaultSubdued,
  zIndex: 0,
}));

export const StepItem = styled(Box)(({ theme, active }) => ({
  display: 'flex',
  flexDirection: 'row',
  alignItems: 'center',
  zIndex: active ? 2 : 1,
  backgroundColor: theme.palette.background.paper,
  padding: theme.spacing(0, 1),
  position: 'relative',
  paddingBottom: theme.spacing(1),
  width: 'auto',
  minWidth: '180px',
  '&:not(:last-child)': {
    marginRight: theme.spacing(4),
  },
}));

export const StepNumber = styled(Box)(({ theme, active, completed }) => ({
  width: '24px',
  height: '24px',
  minWidth: '24px',
  minHeight: '24px',
  borderRadius: '50%',
  border: `${active ? '1.5px' : '1px'} solid ${completed
    ? theme.palette.background.iconSuccessDefault
    : active
      ? theme.palette.background.iconSuccessDefault
      : theme.palette.border.neutralDefault}`,
  backgroundColor: 'transparent',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  marginRight: theme.spacing(1),
  position: 'relative',
  zIndex: 2,
  boxSizing: 'border-box',
  '@media (max-width: 900px)': {
    width: '22px',
    height: '22px',
    minWidth: '22px',
    minHeight: '22px',
    marginRight: theme.spacing(0.75),
  },
  '@media (max-width: 600px)': {
    width: '20px',
    height: '20px',
    minWidth: '20px',
    minHeight: '20px',
    marginRight: theme.spacing(0.5),
  },
}));

export const StepProgressContainer = styled(Box)(({ theme }) => ({
  width: '100%',
  position: 'relative',
  marginBottom: theme.spacing(3),
}));

export const StepProgressConnector = styled(Box)(({ theme }) => ({
  position: 'absolute',
  bottom: 0,
  left: 0,
  right: 0,
  height: '1.6px',
  backgroundColor: theme.palette.border.neutralDefaultSubdued,
  zIndex: 0,
}));

export const ActiveStepIndicator = styled(Box)(({ theme, width }) => ({
  position: 'absolute',
  bottom: 0,
  left: 0,
  width: width,
  height: '1.6px',
  backgroundColor: theme.palette.background.iconSuccessDefault,
  zIndex: 1,
}));

export const StepsContainer = styled(Box)(({ theme }) => ({
  display: 'flex',
  justifyContent: 'flex-start',
  alignItems: 'center',
  width: '100%',
  position: 'relative',
}));

export const StepContainer = styled(Box)(({ theme, width }) => ({
  display: 'flex',
  flexDirection: 'row',
  alignItems: 'center',
  padding: theme.spacing(0, 1),
  paddingBottom: theme.spacing(1),
  width: width,
  zIndex: 2,
  '@media (max-width: 900px)': {
    padding: theme.spacing(0, 0.75),
    paddingBottom: theme.spacing(1),
  },
  '@media (max-width: 600px)': {
    padding: theme.spacing(0, 0.5),
    paddingBottom: theme.spacing(1),
  },
}));