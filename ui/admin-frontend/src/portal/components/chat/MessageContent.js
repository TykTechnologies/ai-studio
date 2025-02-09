import React, { useState } from 'react';
import { Box, TextField, IconButton } from '@mui/material';
import EditIcon from '@mui/icons-material/Edit';
import SaveIcon from '@mui/icons-material/Save';
import CancelIcon from '@mui/icons-material/Cancel';
import MarkdownMessage from './MarkdownMessage';
import pubClient from '../../../admin/utils/pubClient';
import SystemMessage from './SystemMessage';

const MessageContent = ({
	content,
	messageIndex,
	expandedGroups,
	toggleGroup,
	messageId,
	messageType,
	sessionId,
	onEditSuccess,
}) => {
	const [isEditing, setIsEditing] = useState(false);
	const [editText, setEditText] = useState(content || '');

	// Only user messages can be edited:
	const handleEditClick = () => {
		setIsEditing(true);
	};

	const handleCancelClick = () => {
		setIsEditing(false);
		setEditText(content);
	};

	const handleSaveClick = async () => {
		if (!sessionId || !messageId) {
			setIsEditing(false);
			return;
		}
		try {
			await pubClient.put(`/common/chat-sessions/${sessionId}/messages/${messageId}`, {
				new_content: editText,
			});
			setIsEditing(false);
			if (onEditSuccess) {
				onEditSuccess();
			}
		} catch (err) {
			console.error('Error editing message:', err);
			setIsEditing(false);
		}
	};

	// If user message is in editing mode:
	if (messageType === 'user' && isEditing) {
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

	// For system messages and messages containing system/context blocks
	if (messageType === 'system' || content.includes(':::system') || content.includes('[CONTEXT]')) {
		const groupId = messageType === 'system' ? `system-${messageIndex}` : `system-message-${messageIndex}`;
		return (
			<SystemMessage
				content={content}
				groupId={groupId}
				isExpanded={expandedGroups[groupId]}
				toggleGroup={() => toggleGroup(groupId)}
			/>
		);
	}

	return (
		<Box
			sx={{
				position: 'relative',
				'&:hover .edit-button': {
					visibility: messageType === 'user' ? 'visible' : 'hidden',
				},
			}}
		>
			{messageType === 'user' && (
				<IconButton
					className="edit-button"
					size="small"
					onClick={handleEditClick}
					sx={{
						position: 'absolute',
						top: 0,
						right: 0,
						visibility: 'hidden',
						zIndex: 100,
						opacity: 0.8,
					}}
				>
					<EditIcon fontSize="small" />
				</IconButton>
			)}
			<MarkdownMessage content={content} />
		</Box>
	);
};

export default MessageContent;
