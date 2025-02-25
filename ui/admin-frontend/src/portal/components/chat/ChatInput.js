import React from 'react';
import { Box, TextField, IconButton, Chip } from '@mui/material';
import SendIcon from '@mui/icons-material/Send';
import AttachFileIcon from '@mui/icons-material/AttachFile';
import TextareaAutosize from '@mui/material/TextareaAutosize';
import { useDropzone } from 'react-dropzone';

const ChatInput = ({
	inputMessage,
	setInputMessage,
	handleSendMessage,
	isConnected,
	uploadedFiles,
	setUploadedFiles,
	onDrop,
	isUploading,
	renderUploadIndicator
}) => {
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
			sx={{ p: 1, borderTop: 0, minHeight: '64px', position: 'relative' }}
			{...getRootProps()}
		>
			<input {...getInputProps()} />
			<TextField
				fullWidth
				variant="outlined"
				placeholder="Type your message here... (Enter to send, Shift+Enter for new line)"
				sx={{
					'& .MuiOutlinedInput-root': {
						'& fieldset': {
							border: '1px solid transparent',
							borderImage: 'linear-gradient(163.33deg, #23E2C2 46.22%, #5900CB 161.35%)',
							borderImageSlice: 1
						}
					}
				}}
				value={inputMessage}
				onChange={(e) => setInputMessage(e.target.value)}
				onKeyDown={handleKeyDown}
				disabled={!isConnected}
				multiline
				minRows={1}
				maxRows={4}
				InputProps={{
					inputComponent: TextareaAutosize,
					endAdornment: (
						<Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
							{uploadedFiles.length > 0 && (
								<Chip
									icon={<AttachFileIcon />}
									label={uploadedFiles.length}
									size="small"
									onDelete={() => setUploadedFiles([])}
								/>
							)}
							{renderUploadIndicator()}
							<IconButton onClick={open} size="small">
								<AttachFileIcon />
							</IconButton>
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
