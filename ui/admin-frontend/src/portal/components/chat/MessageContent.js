import React, { useState } from 'react';
import { Box, TextField, IconButton, Typography } from '@mui/material';
import EditIcon from '@mui/icons-material/Edit';
import SaveIcon from '@mui/icons-material/Save';
import CancelIcon from '@mui/icons-material/Cancel';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined';
import MarkdownMessage from './MarkdownMessage';
import pubClient from '../../../admin/utils/pubClient';

const extractSystemBlocks = (content) => {
	const segments = [];
	let lastIndex = 0;

	// Combined regex for both system and context blocks
	const regex = /(?:(?::{3}|%%%)system\s*([\s\S]*?)(?::{3}|%%%)|(?:\[CONTEXT\]\s*([\s\S]*?)\s*\[\/CONTEXT\]))/gi;
	let match;

	while ((match = regex.exec(content)) !== null) {
		const matchStart = match.index;
		const matchEnd = regex.lastIndex;

		// Add any text before this match as normal content
		if (matchStart > lastIndex) {
			segments.push({
				type: 'content',
				text: content.slice(lastIndex, matchStart).trim()
			});
		}

		// Determine if it's a system message or context block
		const systemMatch = match[1];
		const contextMatch = match[2];

		if (systemMatch) {
			// System message - match[1] contains the content
			segments.push({ type: 'system', text: match[1].trim() });
		} else if (contextMatch) {
			// Context block - match[2] contains the content
			segments.push({ type: 'context', text: match[2].trim() });
		}

		lastIndex = matchEnd;
	}

	// Add any remaining content
	if (lastIndex < content.length) {
		const remainingText = content.slice(lastIndex).trim();
		if (remainingText) {
			segments.push({ type: 'content', text: remainingText });
		}
	}

	return segments;
};

const SystemBlock = ({ messages, groupId, isExpanded, toggleGroup }) => {
	const firstMsg = messages[0];
	const hasMultipleMessages = messages.length > 1;
	const isError = firstMsg.toLowerCase().includes('error:');

	return (
		<Box
			sx={{
				backgroundColor: '#E0F7F6',
				border: '1px solid #e9ecef',
				borderRadius: '10px',
				boxShadow: '0px 4px 8px rgba(0, 0, 0, 0.1)',
				padding: '12px 12px',
				margin: '10px 0',
				color: '#000000',
				fontFamily: 'monospace',
				cursor: hasMultipleMessages ? 'pointer' : 'default'
			}}
			onMouseDown={hasMultipleMessages ? (e) => {
				e.preventDefault();
				e.stopPropagation();
				toggleGroup(groupId);
				return false
			} : undefined}
		>
			<Box
				sx={{
					display: 'flex',
					alignItems: 'center',
					gap: 1,
					backgroundColor: isError ? '#FEE2E2' : 'transparent',
					color: isError ? '#DC2626' : 'inherit',
					padding: '4px 8px',
					borderRadius: '4px'
				}}
			>
				<SmartToyOutlinedIcon
					sx={{
						fontSize: '1rem',
						color: isError ? '#DC2626' : '#666'
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
						borderTop: '1px solid rgba(0, 0, 0, 0.1)',
						pt: 1,
						color: '#666',
						fontSize: '0.8rem'
					}}
				>
					<Typography variant="caption">
						{isExpanded ? 'Click to collapse' : `${messages.length - 1} more messages...`}
					</Typography>
					<KeyboardArrowDownIcon
						sx={{
							transform: isExpanded ? 'rotate(180deg)' : 'none',
							transition: 'transform 0.2s'
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
									mt: 1
								}}
								onClick={(e) => e.stopPropagation()}
							>
								<SmartToyOutlinedIcon
									sx={{
										fontSize: '1rem',
										color: msgIsError ? '#DC2626' : '#666'
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

const ContextBlock = ({ content, groupId, isExpanded, toggleGroup }) => {
	return (
		<Box
			sx={{
				backgroundColor: '#F5F5F5',
				border: '1px solid #e9ecef',
				borderRadius: '10px',
				boxShadow: '0px 4px 8px rgba(0, 0, 0, 0.1)',
				padding: '12px 12px',
				margin: '10px 0',
				color: '#666',
				fontFamily: 'monospace',
				cursor: 'pointer'
			}}
			onMouseDown={(e) => {
				e.preventDefault();
				toggleGroup(groupId);
			}}
		>
			<Box
				sx={{
					display: 'flex',
					alignItems: 'center',
					gap: 1
				}}
			>
				<Typography
					variant="caption"
					sx={{
						fontWeight: 'bold',
						color: '#666'
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
					borderTop: '1px solid rgba(0, 0, 0, 0.1)',
					pt: 1,
					color: '#666',
					fontSize: '0.8rem'
				}}
			>
				<Typography variant="caption">
					{isExpanded ? 'Click to collapse' : 'Click to show context'}
				</Typography>
				<KeyboardArrowDownIcon
					sx={{
						transform: isExpanded ? 'rotate(180deg)' : 'none',
						transition: 'transform 0.2s'
					}}
				/>
			</Box>

			{isExpanded && (
				<Box sx={{ mt: 1 }}>
					<MarkdownMessage content={content} />
				</Box>
			)}
		</Box>
	);
};

const MessageContent = ({
	content,
	messageIndex,
	expandedGroups,
	toggleGroup,
	messageId,
	messageType,
	sessionId,
	onEditSuccess
}) => {
	const [isEditing, setIsEditing] = useState(false);
	const [editText, setEditText] = useState(content || '');

	const canEdit = messageType === 'user' && !String(messageId).startsWith('temp_');

	const handleEditClick = () => {
		if (!canEdit) return;
		setIsEditing(true);
	};

	const handleCancelClick = () => {
		setIsEditing(false);
		setEditText(content);
	};

	const handleSaveClick = async () => {
		if (!sessionId || !canEdit) {
			setIsEditing(false);
			return;
		}
		try {
			await pubClient.put(
				`/common/chat-sessions/${sessionId}/messages/${messageId}`,
				{
					new_content: editText
				},
				{
					headers: {
						'Cache-Control': 'no-cache',
						Pragma: 'no-cache'
					}
				}
			);
			setIsEditing(false);
			if (onEditSuccess) {
				onEditSuccess();
			}
		} catch (err) {
			console.error('Error editing message:', err);
			setIsEditing(false);
		}
	};

	if (canEdit && isEditing) {
		return (
			<Box>
				<TextField
					variant="outlined"
					fullWidth
					multiline
					value={editText}
					onChange={(e) => setEditText(e.target.value)}
					sx={{ mb: 1 }}
				/>
				<Box sx={{ display: 'flex', gap: 1 }}>
					<IconButton color="success" onClick={handleSaveClick}>
						<SaveIcon />
					</IconButton>
					<IconButton color="inherit" onClick={handleCancelClick}>
						<CancelIcon />
					</IconButton>
				</Box>
			</Box>
		);
	}

	const baseGroupId = `message-${messageIndex}`;

	// Handle system message type or messages containing system/context blocks
	if (messageType === 'system' || content.includes(':::system') || content.includes('[CONTEXT]')) {
		const segments = messageType === 'system'
			? [{ type: 'system', text: content.replace(/(?::{3}|%%%)system\s*/i, '').replace(/(?::{3}|%%%)/g, '').trim() }]
			: extractSystemBlocks(content);

		// Group consecutive system messages
		const processedSegments = [];
		let currentSystemGroup = [];

		segments.forEach((segment, index) => {
			if (segment.type === 'system') {
				currentSystemGroup.push(segment.text);
			} else {
				if (currentSystemGroup.length > 0) {
					processedSegments.push({
						type: 'system-group',
						messages: currentSystemGroup
					});
					currentSystemGroup = [];
				}
				processedSegments.push(segment);
			}
		});

		if (currentSystemGroup.length > 0) {
			processedSegments.push({
				type: 'system-group',
				messages: currentSystemGroup
			});
		}

		return (
			<Box sx={{ position: 'relative' }}>
				<Box
					sx={{
						position: 'relative',
						'&:hover .edit-button': {
							opacity: 1,
							visibility: 'visible'
						}
					}}
				>
					{processedSegments.map((segment, idx) => {
						if (segment.type === 'system-group') {
							return (
								<SystemBlock
									key={`system-${idx}`}
									messages={segment.messages}
									groupId={`${baseGroupId}-system-${idx}`}
									isExpanded={expandedGroups[`${baseGroupId}-system-${idx}`] || false}
									toggleGroup={toggleGroup}
								/>
							);
						} else if (segment.type === 'context') {
							return (
								<ContextBlock
									key={`context-${idx}`}
									content={segment.text}
									groupId={`${baseGroupId}-context-${idx}`}
									isExpanded={expandedGroups[`${baseGroupId}-context-${idx}`] || false}
									toggleGroup={toggleGroup}
								/>
							);
						} else {
							return (
								<Box key={`content-${idx}`} sx={{ my: 1 }}>
									<MarkdownMessage content={segment.text} />
								</Box>
							);
						}
					})}
					{canEdit && (
						<IconButton
							className="edit-button"
							size="small"
							onClick={handleEditClick}
							sx={{
								position: 'absolute',
								top: 0,
								right: 0,
								visibility: 'hidden',
								opacity: 0,
								zIndex: 100,
								transition: 'opacity 0.2s ease-in-out, visibility 0.2s ease-in-out'
							}}
						>
							<EditIcon fontSize="small" />
						</IconButton>
					)}
				</Box>
			</Box>
		);
	}

	// Regular message (not system/context)
	return (
		<Box
			sx={{
				position: 'relative',
				'&:hover .edit-button': {
					opacity: 1,
					visibility: 'visible'
				}
			}}
		>
			<MarkdownMessage content={content} />
			{canEdit && (
				<IconButton
					className="edit-button"
					size="small"
					onClick={handleEditClick}
					sx={{
						position: 'absolute',
						top: 0,
						right: 0,
						visibility: 'hidden',
						opacity: 0,
						zIndex: 100,
						transition: 'opacity 0.2s ease-in-out, visibility 0.2s ease-in-out'
					}}
				>
					<EditIcon fontSize="small" />
				</IconButton>
			)}
		</Box>
	);
};

export default MessageContent;
