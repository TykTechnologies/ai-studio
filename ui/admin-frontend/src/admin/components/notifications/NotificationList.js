import React, { useState, useEffect } from 'react';
import {
	List,
	ListItem,
	ListItemText,
	Typography,
	Paper,
	Container,
	IconButton,
	ListItemSecondaryAction,
	Box,
	Button,
} from '@mui/material';
import DoneIcon from '@mui/icons-material/Done';
import DoneAllIcon from '@mui/icons-material/DoneAll';
import axios from 'axios';
import { useNavigate } from 'react-router-dom';
import { useNotifications } from '../../context/NotificationContext';

const PREVIEW_LENGTH = 160;

// Content is plain text now; show a short preview and let the item open its object.
const getPreview = (content) => {
	const text = (content || '').replace(/\s+/g, ' ').trim();
	if (text.length <= PREVIEW_LENGTH) return text;
	return `${text.slice(0, PREVIEW_LENGTH).trimEnd()}…`;
};

const getLink = (notification) =>
	notification.Link ?? notification.link ?? notification.attributes?.link ?? '';

const NotificationList = () => {
	const navigate = useNavigate();
	const [notifications, setNotifications] = useState([]);
	const { markAsRead, markAllAsRead } = useNotifications();

	const fetchNotifications = async () => {
		try {
			const response = await axios.get('/common/api/v1/notifications');
			setNotifications(response.data);
		} catch (error) {
			console.error('Error fetching notifications:', error);
		}
	};

	useEffect(() => {
		fetchNotifications();
	}, []);

	const handleMarkAsRead = async (id) => {
		const success = await markAsRead(id);
		if (success) {
			// Update the local state to mark the notification as read
			setNotifications(notifications.map(notification =>
				notification.ID === id
					? { ...notification, Read: true }
					: notification
			));
		}
	};

	const handleNotificationClick = async (notification) => {
		if (!notification.Read) {
			await handleMarkAsRead(notification.ID);
		}
		const link = getLink(notification);
		if (link) {
			if (/^https?:\/\//i.test(link)) {
				window.open(link, '_blank', 'noopener');
			} else {
				navigate(link);
			}
		}
	};

	const handleMarkAllAsRead = async () => {
		const success = await markAllAsRead();
		if (success) {
			// Update the local state to mark all notifications as read
			setNotifications(notifications.map(notification => ({
				...notification,
				Read: true
			})));
		}
	};

	const hasUnread = notifications.some(notification => !notification.Read);

	return (
		<Container maxWidth="md" sx={{ mt: 4 }}>
			<Paper>
				<Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
					<Typography variant="headingXLarge">
						Notifications
					</Typography>
					{hasUnread && (
						<Button
							variant="outlined"
							size="small"
							startIcon={<DoneAllIcon />}
							onClick={handleMarkAllAsRead}
						>
							Mark All as Read
						</Button>
					)}
				</Box>
				<List>
					{notifications.map((notification) => (
						<ListItem
							key={notification.ID}
							onClick={() => handleNotificationClick(notification)}
							sx={{
								cursor: 'pointer',
								backgroundColor: notification.Read ? 'transparent' : 'rgba(0, 0, 0, 0.04)',
							}}
						>
							<ListItemText
								primary={notification.Title}
								secondary={getPreview(notification.Content)}
								secondaryTypographyProps={{ title: notification.Content || '' }}
							/>
							{!notification.Read && (
								<ListItemSecondaryAction>
									<IconButton
										edge="end"
										aria-label="mark as read"
										onClick={(e) => {
											e.stopPropagation();
											handleMarkAsRead(notification.ID);
										}}
									>
										<DoneIcon />
									</IconButton>
								</ListItemSecondaryAction>
							)}
						</ListItem>
					))}
					{notifications.length === 0 && (
						<ListItem>
							<ListItemText
								primary="No notifications"
								secondary="You don't have any notifications at the moment"
							/>
						</ListItem>
					)}
				</List>
			</Paper>
		</Container>
	);
};

export default NotificationList;
