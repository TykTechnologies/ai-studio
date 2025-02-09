import React from 'react';
import { Box, Typography } from '@mui/material';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

/**
	* groupSystemMessages reimplements the older code's logic:
	* 1. Splits content by any chunk that looks like:
	*    - ":::system ... :::" or "%%%system ... %%%"
	*    - "[CONTEXT] ... [/CONTEXT]"
	*    capturing the entire block including the system/context markers,
	*    then grouping consecutive system lines or context blocks.
	*/
function groupSystemMessages(segments) {
	const groupedSegments = [];
	let currentSystemGroup = [];

	segments.forEach((segment) => {
		if (segment.match(/(?::::|%%%)system[\s\S]*?(?::::|%%%)/)) {
			// It's a system segment, remove the markers and push to currentSystemGroup
			const cleaned = segment
				.replace(/(?::::|%%%)system\s*/i, '')
				.replace(/:{3,}$|(%%%)+$/i, '')
				.trim();
			currentSystemGroup.push(cleaned);
		} else if (segment.match(/\[CONTEXT\][\s\S]*?\[\/CONTEXT\]/i)) {
			// It's a context block
			const contextContent = segment
				.replace(/\[CONTEXT\]\s*/i, '')
				.replace(/\[\/CONTEXT\]/i, '')
				.trim();
			// If we already have system lines accumulating, push them first
			if (currentSystemGroup.length > 0) {
				groupedSegments.push({ type: 'system-group', messages: currentSystemGroup });
				currentSystemGroup = [];
			}
			groupedSegments.push({ type: 'context-group', messages: [contextContent] });
		} else if (segment.trim()) {
			// It's a normal content chunk
			// If there's an existing system group, flush it
			if (currentSystemGroup.length > 0) {
				groupedSegments.push({ type: 'system-group', messages: currentSystemGroup });
				currentSystemGroup = [];
			}
			groupedSegments.push({ type: 'content', content: segment.trim() });
		}
	});

	// If we ended with a system group pending
	if (currentSystemGroup.length > 0) {
		groupedSegments.push({ type: 'system-group', messages: currentSystemGroup });
	}

	return groupedSegments;
}

/**
	* SystemMessage component is used whenever messageType === 'system' or
	* the message content has ":::system" or "[CONTEXT]" markers inside it.
	*/
const SystemMessage = ({ content, groupId, isExpanded, toggleGroup }) => {
	// We split the content with the same (older) approach so everything is recognized properly
	// We use capturing group to keep the delimiters for further grouping.
	const segments = content.split(
		/((?::::|%%%)system[\s\S]*?(?::::|%%%)|\[CONTEXT\][\s\S]*?\[\/CONTEXT\])/g
	);

	const groupedSegments = groupSystemMessages(segments);

	return (
		<Box>
			{groupedSegments.map((segment, index) => {
				if (segment.type === 'system-group') {
					const messages = segment.messages;
					const firstMsg = messages[0];
					const hasMultipleMessages = messages.length > 1;
					const isError = firstMsg.toLowerCase().includes('error:');

					return (
						<Box
							key={`${groupId}-${index}`}
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
							<Box
								sx={{
									display: 'flex',
									alignItems: 'center',
									gap: 1,
									backgroundColor: isError ? '#FEE2E2' : 'transparent',
									color: isError ? '#DC2626' : 'inherit',
									padding: '4px 8px',
									borderRadius: '4px',
								}}
							>
								<SmartToyOutlinedIcon
									sx={{
										fontSize: '1rem',
										color: isError ? '#DC2626' : '#666',
									}}
								/>
								{firstMsg}
							</Box>

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

							{isExpanded && hasMultipleMessages && (
								<Box sx={{ mt: 1 }}>
									{messages.slice(1).map((message, msgIndex) => {
										const msgIsError = message.toLowerCase().includes('error:');
										return (
											<Box
												key={msgIndex}
												sx={{
													display: 'flex',
													alignItems: 'center',
													gap: 1,
													backgroundColor: msgIsError ? '#FEE2E2' : 'transparent',
													color: msgIsError ? '#DC2626' : 'inherit',
													padding: '4px 8px',
													borderRadius: '4px',
													mt: 1,
												}}
												onClick={(e) => e.stopPropagation()}
											>
												<SmartToyOutlinedIcon
													sx={{
														fontSize: '1rem',
														color: msgIsError ? '#DC2626' : '#666',
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
				} else if (segment.type === 'context-group') {
					return (
						<Box
							key={`${groupId}-ctx-${index}`}
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
										{segment.messages[0]}
									</ReactMarkdown>
								</Box>
							)}
						</Box>
					);
				} else if (segment.type === 'content') {
					// Plain text segment
					return <Box key={`content-${index}`}>{segment.content}</Box>;
				}
				return null;
			})}
		</Box>
	);
};

export default SystemMessage;
