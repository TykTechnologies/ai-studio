import React, { useRef } from 'react';
import { Box, TextField, IconButton, Chip } from '@mui/material';
import SendIcon from '@mui/icons-material/Send';
import AttachFileIcon from '@mui/icons-material/AttachFile';
import { useDropzone } from 'react-dropzone';
import PromptTemplateSelector from './PromptTemplateSelector';

const ChatInput = ({
	inputMessage,
	setInputMessage,
	handleSendMessage,
	isConnected,
	uploadedFiles,
	setUploadedFiles,
	onDrop,
	isUploading,
	renderUploadIndicator,
	chatId,
	messages = [],
	hideFileUpload = false
}) => {
	// Use a ref for the input element to prevent unnecessary re-renders
	const inputRef = useRef(null);

	const { getRootProps, getInputProps, isDragActive, open } = useDropzone({
		onDrop,
		noClick: true,
		noKeyboard: true,
	});

	const handleKeyDown = (e) => {
		if (e.key === 'Enter') {
			if (e.shiftKey || e.metaKey || e.ctrlKey) {
				return;
			}
			e.preventDefault();
			handleSendMessage(e);
		}
	};

	return (
		<Box
			component="form"
			onSubmit={handleSendMessage}
			sx={{
				mb: 2,
				position: 'relative',
				mx: '1px'
			}}
			{...getRootProps()}
		>
			<input {...getInputProps()} />
			<Box sx={{
				position: 'relative',
				'&:before': {
					content: '""',
					position: 'absolute',
					inset: -1,
					padding: '1px',
					borderRadius: '8px',
					background: 'linear-gradient(163.33deg, #23E2C2 46.22%, #5900CB 161.35%)',
					WebkitMask: 'linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0)',
					WebkitMaskComposite: 'xor',
					pointerEvents: 'none', // This prevents interference with text selection
				}
			}}>
				<TextField
					ref={inputRef}
					fullWidth
					variant="outlined"
					placeholder="Type your message here... (Enter to send, Shift+Enter for new line)"
					value={inputMessage}
					onChange={(e) => setInputMessage(e.target.value)}
					onKeyDown={handleKeyDown}
					disabled={!isConnected}
					multiline
					minRows={1}
					maxRows={10}
					sx={{
						'& .MuiOutlinedInput-root': {
							minHeight: '92px',
							borderRadius: '8px',
							fontSize: '1rem',
							backgroundColor: 'background.paper',
							position: 'relative',
							'& .MuiOutlinedInput-notchedOutline': {
								border: 'none'
							},
						},
						'& textarea': {
							minHeight: '60px',
						}
					}}
					InputProps={{
						endAdornment: (
							<Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexShrink: 0 }}>
								{!hideFileUpload && uploadedFiles.length > 0 && (
									<Chip
										icon={<AttachFileIcon />}
										label={uploadedFiles.length}
										size="small"
										onDelete={() => setUploadedFiles([])}
									/>
								)}
								{!hideFileUpload && renderUploadIndicator()}
								{!hideFileUpload && (
									<IconButton onClick={open} size="small">
										<AttachFileIcon />
									</IconButton>
								)}
								<IconButton
									onClick={handleSendMessage}
									disabled={!isConnected || (!inputMessage.trim() && uploadedFiles.length === 0)}
									size="small"
								>
									<SendIcon />
								</IconButton>
							</Box>
						),
					}}
				/>
			</Box>
			{messages.length === 0 && (
				<PromptTemplateSelector
					chatId={chatId}
					onSelectTemplate={(template) => setInputMessage(template)}
					disabled={!isConnected}
					sx={{ mt: 2 }}
				/>
			)}
			{isDragActive && (
				<Box
					sx={{
						position: 'absolute',
						top: 0,
						left: 0,
						right: 0,
						bottom: 0,
						backgroundColor: 'rgba(0, 0, 0, 0.1)',
						display: 'flex',
						alignItems: 'center',
						justifyContent: 'center',
					}}
				>
					<div style={{ color: '#666' }}>Drop files here to upload</div>
				</Box>
			)}
		</Box>
	);
};

export default ChatInput;
