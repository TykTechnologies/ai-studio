import React from 'react';
import { Box } from '@mui/material';
import FloatingSection from '../FloatingSection';

const ChatSidebar = ({
	currentlyUsing,
	databases,
	tools,
	showTools,
	removeFromCurrentlyUsing,
	addToCurrentlyUsing
}) => {
	return (
		<Box
			sx={{
				display: 'flex',
				flexDirection: 'column',
				height: '100%',
				gap: 1,
				p: 1,
				overflowY: 'auto',
			}}
		>
			<FloatingSection
				key="currentlyUsing"
				title="Currently Using..."
				items={currentlyUsing}
				onRemove={removeFromCurrentlyUsing}
				emptyText="Click + on tools and databases to use them in the chat"
			/>
			<FloatingSection
				key="databases"
				title="Databases"
				items={databases}
				onAdd={addToCurrentlyUsing}
			/>
			{showTools && (
				<FloatingSection
					key="tools"
					title="Tools"
					items={tools}
					onAdd={addToCurrentlyUsing}
				/>
			)}
		</Box>
	);
};

export default ChatSidebar;
