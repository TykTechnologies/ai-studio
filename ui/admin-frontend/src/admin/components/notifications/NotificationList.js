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
import NotificationMarkdown from './NotificationMarkdown';
import DoneIcon from '@mui/icons-material/Done';
import DoneAllIcon from '@mui/icons-material/DoneAll';
import axios from 'axios';
import { useNotifications } from '../../context/NotificationContext';

const NotificationList = () => {
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
								secondary={
									<Box sx={{ '& img': { maxWidth: '100%' }, '& pre': { overflow: 'auto' } }}>
										<NotificationMarkdown>
											{notification.Content}
										</NotificationMarkdown>
									</Box>
								}
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
