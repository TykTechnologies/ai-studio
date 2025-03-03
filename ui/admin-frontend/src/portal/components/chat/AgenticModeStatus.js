import React from 'react';
import { Tooltip, IconButton } from '@mui/material';
import SmartToyIcon from '@mui/icons-material/SmartToy';

const AgenticModeStatus = ({ isEnabled, toggleAgenticMode }) => {
	return (
		<Tooltip title={isEnabled ? "Agentic Mode Enabled" : "Enable Agentic Mode"}>
			<IconButton
				onClick={toggleAgenticMode}
				sx={{
					color: isEnabled ? 'primary.main' : 'text.secondary',
					'&:hover': {
						color: isEnabled ? 'primary.dark' : 'text.primary',
					}
				}}
			>
				<SmartToyIcon />
			</IconButton>
		</Tooltip>
	);
};

export default AgenticModeStatus;
