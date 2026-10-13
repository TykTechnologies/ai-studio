import React from 'react';
import { Link as RouterLink } from 'react-router-dom';
import { Box, styled, Button, Link, Typography } from '@mui/material';
import { SecondaryOutlineButton } from '../../styles/sharedStyles';

const CardContainer = styled(Box)(({ theme }) => ({
  height: 'auto',
  minHeight: '214px',
  minWidth: '345px',
  width: '100%',
  border: `1px solid ${theme.palette.border.neutralDefault}`,
  borderRadius: '8px',
  display: 'flex',
  flexDirection: 'column',
  overflow: 'hidden',
  backgroundColor: theme.palette.background.paper,
  boxShadow: '4px 4px 8px 0px rgba(9, 9, 35, 0.08)',
  boxSizing: 'border-box',
}));

const CardContent = styled(Box)(({ theme }) => ({
  flex: 1,
  padding: theme.spacing(2),
  overflow: 'auto',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'flex-start',
  width: '100%',
  boxSizing: 'border-box',
}));

const CardActions = styled(Box)(({ theme }) => ({
  padding: theme.spacing(1.5),
  display: 'flex',
  justifyContent: 'flex-end',
  gap: theme.spacing(1),
  borderTop: `1px solid ${theme.palette.border.neutralDefaultSubdued}`,
  flexWrap: 'wrap',
  width: '100%',
  boxSizing: 'border-box',
}));

const ActionButton = styled(Button)(({ theme }) => ({
  padding: '2px 8px',
}));

const SecondaryActionButton = styled(SecondaryOutlineButton)(({ theme }) => ({
  padding: '2px 8px',
}));

const PrerequisiteLine = styled(Box)(({ theme }) => ({
  padding: theme.spacing(0, 2, 1.5),
}));

// prerequisite: { text, links: [{ label, to }] } -- what the feature needs
// before its call to action can succeed, with links to where it is set up.
const BasicCard = ({ children, primaryAction, secondaryAction, prerequisite }) => {
  return (
    <CardContainer>
      <CardContent>{children}</CardContent>
      {prerequisite && (
        <PrerequisiteLine data-testid="card-prerequisite">
          <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
            <strong>Needs:</strong> {prerequisite.text}
            {(prerequisite.links || []).map((link, i) => (
              <React.Fragment key={link.to}>
                {i === 0 ? ' ' : ' · '}
                <Link component={RouterLink} to={link.to}>
                  {link.label}
                </Link>
              </React.Fragment>
            ))}
          </Typography>
        </PrerequisiteLine>
      )}
      <CardActions>
        {secondaryAction && (
          <SecondaryActionButton
            onClick={secondaryAction.onClick}
            disabled={secondaryAction.disabled || false}
          >
            {secondaryAction.label}
          </SecondaryActionButton>
        )}
        {primaryAction && (
          <ActionButton
            onClick={primaryAction.onClick}
            disabled={primaryAction.disabled || false}
          >
            {primaryAction.label}
          </ActionButton>
        )}
      </CardActions>
    </CardContainer>
  );
};

export default BasicCard;