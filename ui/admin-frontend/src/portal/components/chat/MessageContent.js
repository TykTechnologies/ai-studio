import React, { useState, useEffect } from 'react';
import { Box, TextField, IconButton, Typography } from '@mui/material';
import EditIcon from '@mui/icons-material/Edit';
import SaveIcon from '@mui/icons-material/Save';
import CancelIcon from '@mui/icons-material/Cancel';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined';
import MarkdownMessage from './MarkdownMessage';
import pubClient from '../../../admin/utils/pubClient';

const MessageAvatar = ({ messageType, userName }) => (
  <Box
    sx={{
      width: 35,
      height: 35,
      borderRadius: '50%',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      bgcolor: messageType === 'user' ? 'background.surfaceBrandHovered' : 'background.surfaceDefaultBubble'
    }}
  >
    <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
      {messageType === 'user' ? userName?.charAt(0).toUpperCase() || 'U' : 'AI'}
    </Typography>
  </Box>
);

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

	// Get error type from the first message if it's an error
	const getErrorType = (msg) => {
		if (!msg.toLowerCase().includes('error:')) return null;
		if (msg.toLowerCase().includes('api error')) return 'api';
		if (msg.toLowerCase().includes('connection error')) return 'connection';
		if (msg.toLowerCase().includes('authentication error')) return 'auth';
		return 'system';
	};

	const errorType = getErrorType(firstMsg);

	// Get error styling based on type
	const getErrorStyling = (type) => {
		switch (type) {
			case 'api':
				return {
					bg: '#FEF3F2',
					border: '#FCA5A5',
					color: '#B91C1C',
					icon: '🌐'
				};
			case 'connection':
				return {
					bg: '#FEF9C3',
					border: '#FDE047',
					color: '#854D0E',
					icon: '🔌'
				};
			case 'auth':
				return {
					bg: '#F3E8FF',
					border: '#C084FC',
					color: '#6B21A8',
					icon: '🔒'
				};
			default:
				return {
					bg: '#FEE2E2',
					border: '#FCA5A5',
					color: '#DC2626',
					icon: '⚠️'
				};
		}
	};

	const errorStyle = isError ? getErrorStyling(errorType) : null;

	return (
		<Box
			sx={{
				backgroundColor: isError ? errorStyle.bg : '#E0F7F6',
				border: `1px solid ${isError ? errorStyle.border : '#e9ecef'}`,
				borderRadius: '10px',
				boxShadow: '0px 4px 8px rgba(0, 0, 0, 0.1)',
				padding: '12px 12px',
				margin: '10px 0',
				color: isError ? errorStyle.color : '#000000',
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
					flexDirection: 'column',
					gap: 1
				}}
			>
				<Box
					sx={{
						display: 'flex',
						alignItems: 'center',
						gap: 1,
						backgroundColor: 'transparent',
						padding: '4px 8px',
						borderRadius: '4px'
					}}
				>
					{isError ? (
						<Typography
							component="span"
							sx={{
								fontSize: '1.2rem',
								lineHeight: 1
							}}
						>
							{errorStyle.icon}
						</Typography>
					) : (
						<SmartToyOutlinedIcon
							sx={{
								fontSize: '1rem',
								color: '#666'
							}}
						/>
					)}
					{isError ? (
						<Box>
							<Typography
								variant="subtitle2"
								sx={{
									fontWeight: 'bold',
									color: errorStyle.color
								}}
							>
								{firstMsg.split('\n')[0]} {/* Show title */}
							</Typography>
							{firstMsg.split('\n').slice(1).map((line, i) => (
								<Typography
									key={i}
									variant="body2"
									sx={{
										mt: 0.5,
										color: line.startsWith('[Details:') ? 'text.secondary' : 'inherit',
										fontSize: line.startsWith('[Details:') ? '0.85em' : 'inherit'
									}}
								>
									{line}
								</Typography>
							))}
						</Box>
					) : (
						firstMsg
					)}
				</Box>
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
						const msgErrorType = msgIsError ? getErrorType(message) : null;
						const msgStyle = msgIsError ? getErrorStyling(msgErrorType) : null;

						return (
							<Box
								key={msgIndex}
								sx={{
									display: 'flex',
									alignItems: 'flex-start',
									gap: 1,
									backgroundColor: msgIsError ? msgStyle.bg : 'transparent',
									color: msgIsError ? msgStyle.color : 'inherit',
									padding: '8px',
									borderRadius: '4px',
									mt: 1,
									border: msgIsError ? `1px solid ${msgStyle.border}` : 'none'
								}}
								onClick={(e) => e.stopPropagation()}
							>
								{msgIsError ? (
									<Typography
										component="span"
										sx={{
											fontSize: '1.2rem',
											lineHeight: 1
										}}
									>
										{msgStyle.icon}
									</Typography>
								) : (
									<SmartToyOutlinedIcon
										sx={{
											fontSize: '1rem',
											color: '#666',
											mt: '4px'
										}}
									/>
								)}
								<Box sx={{ flex: 1 }}>
									{msgIsError ? (
										message.split('\n').map((line, i) => (
											<Typography
												key={i}
												variant={i === 0 ? 'subtitle2' : 'body2'}
												sx={{
													fontWeight: i === 0 ? 'bold' : 'normal',
													mt: i > 0 ? 0.5 : 0,
													color: line.startsWith('[Details:') ? 'text.secondary' : 'inherit',
													fontSize: line.startsWith('[Details:') ? '0.85em' : 'inherit'
												}}
											>
												{line}
											</Typography>
										))
									) : (
										message
									)}
								</Box>
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
	showSystemMessages,
	onEditSuccess,
	chatId,
	userName
}) => {
	const [isEditing, setIsEditing] = useState(false);
	const [editText, setEditText] = useState('');

	const stripContextWrapper = (text) => {
		const contextMatch = text.match(/\[CONTEXT\][\s\S]*?\[\/CONTEXT\]\s*([\s\S]*)/);
		return contextMatch ? contextMatch[1].trim() : text;
	};

	useEffect(() => {
		if (content) {
			setEditText(stripContextWrapper(content));
		}
	}, [content]);

	const isUserMessage = messageType === 'user';
	const canEdit = isUserMessage;

	const handleEditClick = () => {
		if (!isUserMessage) return;
		setIsEditing(true);
	};


	const handleCancelClick = () => {
		setIsEditing(false);
		setEditText(stripContextWrapper(content));
	};

	const handleSaveClick = async () => {
		if (!sessionId || !isUserMessage) {
			setIsEditing(false);
			return;
		}
		try {
			const isTemp = String(messageId).startsWith('temp_');

			if (isTemp) {
				// For temp IDs, use the index-based editing
				await pubClient.put(
					`/common/chat-sessions/${sessionId}/messages/${messageId}`,
					{
						new_content: {
							role: "human",
							text: editText
						},
						index: messageIndex
					},
					{
						headers: {
							'Cache-Control': 'no-cache',
							Pragma: 'no-cache'
						}
					}
				);
			} else {
				// For real IDs, use the normal edit endpoint
				await pubClient.put(
					`/common/chat-sessions/${sessionId}/messages/${messageId}`,
					{
						new_content: {
							role: "human",
							text: editText
						}
					},
					{
						headers: {
							'Cache-Control': 'no-cache',
							Pragma: 'no-cache'
						}
					}
				);
			}

			setIsEditing(false);
			if (onEditSuccess) {
				onEditSuccess(editText, messageId, isTemp);
			}
		} catch (err) {
			console.error('Error editing message:', err);
			setIsEditing(false);
		}
	};

	if (canEdit && isEditing) {
		return (
			<Box sx={{ px: 6, py: 2 }}>
				<Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
					<TextField
						variant="outlined"
						fullWidth
						multiline
						value={editText}
						onChange={(e) => setEditText(e.target.value)}
					/>
					<Box sx={{ display: 'flex', gap: 1 }}>
						<IconButton
							onClick={handleSaveClick}
							sx={{
								color: '#666',
								'&:hover': {
									color: '#333'
								}
							}}
						>
							<SaveIcon />
						</IconButton>
						<IconButton
							onClick={handleCancelClick}
							sx={{
								color: '#666',
								'&:hover': {
									color: '#333'
								}
							}}
						>
							<CancelIcon />
						</IconButton>
					</Box>
				</Box>
			</Box>
		);
	}

	const baseGroupId = `message-${messageIndex}`;

	// Handle system/error message types or messages containing system/context blocks
	if (messageType === 'system' || content.includes(':::system') || content.includes('[CONTEXT]')) {
		// If system messages are hidden and this is the first system message (not error), return null
		if (!showSystemMessages && messageType === 'system' && messageIndex === 0 && !content.includes('Error:')) {
			return null;
		}

		const segments = messageType === 'system'
			? [{ type: 'system', text: content.replace(/(?::{3}|%%%)system\s*/i, '').replace(/(?::{3}|%%%)/g, '').trim() }]
			: extractSystemBlocks(content);

		// If system messages are hidden, filter out context blocks and first system message
		const visibleSegments = showSystemMessages
			? segments
			: segments.filter(segment => {
				if (segment.type === 'context') return false;
				if (segment.type === 'system' && messageIndex === 0 && !segment.text.includes('Error:')) return false;
				return true;
			});

		// If there are no visible segments after filtering, return null
		if (visibleSegments.length === 0) {
			return null;
		}

		// Group consecutive system messages
		const processedSegments = [];
		let currentSystemGroup = [];

		visibleSegments.forEach((segment, index) => {
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
			<Box
				sx={{
					width: '100%',
					position: 'relative',
					px: 6,
					py: 5,
					display: 'flex',
					gap: 2,
					...(messageType === 'user' && {
						maxWidth: '70%',
						alignSelf: 'end',
						justifyContent: 'end'
					})
				}}
			>
				<MessageAvatar messageType={messageType} userName={userName} />
				<Box
					sx={{
						maxWidth: '70%',
						width: 'fit-content',
						...(messageType === 'user' && {
							bgcolor: 'background.surfaceNeutralDisabled',
							border: '1px solid',
							borderColor: 'border.neutralDefault',
							borderRadius: '8px',
							padding: '12px',
						}),
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
					{isUserMessage && (
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
				width: '100%',
				position: 'relative',
				px: 6,
				py: 5,
				display: 'flex',
				alignItems: 'flex-start',
				gap: 2,
				...(messageType === 'user' && {
					maxWidth: '70%',
					alignSelf: 'end',
					justifyContent: 'end'
				})
			}}
		>
			<MessageAvatar messageType={messageType} userName={userName} />
			<Box
				sx={{
					width: 'fit-content',
					...(messageType === 'user' && {
						bgcolor: 'background.surfaceNeutralDisabled',
						border: '1px solid',
						borderColor: 'border.neutralDefault',
						borderRadius: '8px',
						padding: '12px',
					}),
					...(messageType === 'ai' && {
						borderBottom: '1px solid',
						borderColor: 'border.neutralDefault',
						pb: 2
					  }),
					'&:hover .edit-button': {
						opacity: 1,
						visibility: 'visible'
					}
				}}
			>
				<MarkdownMessage content={content} />
				{isUserMessage && (
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
};

export default MessageContent;
