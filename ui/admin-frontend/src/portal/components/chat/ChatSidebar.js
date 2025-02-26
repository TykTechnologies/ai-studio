import React from 'react';
import { Box, Typography } from '@mui/material';
import FloatingSection from '../FloatingSection';

const ChatSidebar = ({
	currentlyUsing,
	databases,
	tools,
	showTools,
	removeFromCurrentlyUsing,
	addToCurrentlyUsing,
	messages
}) => {
	return (
		<Box
			sx={{
				display: 'flex',
				flexDirection: 'column',
				height: '100%',
				gap: 2,
				p: 3,
				overflowY: 'auto',
				borderLeft: (theme) => `1px solid ${theme.palette.border.neutralDefaultSubdued}`,
			}}
		>
			<Typography variant="bodyLargeMedium">Enhance AI's responses by adding extra context using available data sources and tools.</Typography>
			<FloatingSection
				key="databases"
				title="Data sources"
				items={databases}
				onAdd={addToCurrentlyUsing}
				onRemove={removeFromCurrentlyUsing}
				messages={messages}
			/>
			{showTools && (
				<FloatingSection
					key="tools"
					title="Tools"
					items={tools}
					onAdd={addToCurrentlyUsing}
					onRemove={removeFromCurrentlyUsing}
					messages={messages}
				/>
			)}
		</Box>
	);
};

export default ChatSidebar;
