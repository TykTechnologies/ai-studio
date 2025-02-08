import React from 'react';
import { Box, Typography } from '@mui/material';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

const ContextMessage = ({ content, groupId, isExpanded, toggleGroup }) => {
	return (
		<Box
			sx={{
				backgroundColor: '#F5F5F5',
				border: '1px solid #e9ecef',
				borderRadius: '10px',
				boxShadow: '0px 4px 8px rgba(0, 0, 0, 0.1)',
				padding: '12px 12px',
				margin: '10px 10px',
				color: '#666',
				fontFamily: 'monospace',
				cursor: 'pointer',
			}}
			onClick={() => toggleGroup(groupId)}
		>
			<Box
				sx={{
					display: 'flex',
					alignItems: 'center',
					gap: 1,
				}}
			>
				<Typography
					variant="caption"
					sx={{
						fontWeight: 'bold',
						color: '#666',
					}}
				>
					CONTEXT
				</Typography>
			</Box>

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
					{isExpanded ? 'Click to collapse' : 'Click to show context'}
				</Typography>
				<KeyboardArrowDownIcon
					sx={{
						transform: isExpanded ? 'rotate(180deg)' : 'none',
						transition: 'transform 0.2s',
					}}
				/>
			</Box>

			{isExpanded && (
				<Box sx={{ mt: 1 }}>
					<ReactMarkdown remarkPlugins={[remarkGfm]}>
						{content}
					</ReactMarkdown>
				</Box>
			)}
		</Box>
	);
};

export default ContextMessage;
