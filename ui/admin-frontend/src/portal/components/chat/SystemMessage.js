import React from 'react';
import { Box, Typography } from '@mui/material';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined';

const SystemMessage = ({ messages, groupId, isExpanded, toggleGroup }) => {
	const hasMultipleMessages = messages.length > 1;

	return (
		<Box
			sx={{
				backgroundColor: '#E0F7F6',
				border: '1px solid #e9ecef',
				borderRadius: '10px',
				boxShadow: '0px 4px 8px rgba(0, 0, 0, 0.1)',
				padding: '12px 12px',
				margin: '10px 10px',
				color: '#000000',
				fontFamily: 'monospace',
				cursor: hasMultipleMessages ? 'pointer' : 'default',
			}}
			onClick={hasMultipleMessages ? () => toggleGroup(groupId) : undefined}
		>
			{/* First message is always visible */}
			<Box
				sx={{
					display: 'flex',
					alignItems: 'center',
					gap: 1,
					backgroundColor: messages[0].startsWith('Error:') ? '#FEE2E2' : 'transparent',
					color: messages[0].startsWith('Error:') ? '#DC2626' : 'inherit',
					padding: '4px 8px',
					borderRadius: '4px',
				}}
			>
				<SmartToyOutlinedIcon
					sx={{
						fontSize: '1rem',
						color: messages[0].startsWith('Error:') ? '#DC2626' : '#666',
					}}
				/>
				{messages[0]}
			</Box>

			{/* Show message count and expand/collapse indicator if there are multiple messages */}
			{hasMultipleMessages && (
				<Box
					sx={{
						display: 'flex',
						alignItems: 'center',
						justifyContent: 'space-between',
						mt: 1,
						borderTop: '1px solid rgba(0,0,0,0.1)',
						pt: 1,
						color: '#666',
						fontSize: '0.8rem',
					}}
				>
					<Typography variant="caption">
						{isExpanded ? 'Click to collapse' : `${messages.length - 1} more messages...`}
					</Typography>
					<KeyboardArrowDownIcon
						sx={{
							transform: isExpanded ? 'rotate(180deg)' : 'none',
							transition: 'transform 0.2s',
						}}
					/>
				</Box>
			)}

			{/* Additional messages shown when expanded */}
			{isExpanded && (
				<Box sx={{ mt: 1 }}>
					{messages.slice(1).map((message, msgIndex) => {
						const isError = message.startsWith('Error:');
						return (
							<Box
								key={msgIndex}
								sx={{
									display: 'flex',
									alignItems: 'center',
									gap: 1,
									backgroundColor: isError ? '#FEE2E2' : 'transparent',
									color: isError ? '#DC2626' : 'inherit',
									padding: '4px 8px',
									borderRadius: '4px',
									mt: 1,
								}}
								onClick={(e) => e.stopPropagation()}
							>
								<SmartToyOutlinedIcon
									sx={{
										fontSize: '1rem',
										color: isError ? '#DC2626' : '#666',
									}}
								/>
								{message}
							</Box>
						);
					})}
				</Box>
			)}
		</Box>
	);
};

export default SystemMessage;
