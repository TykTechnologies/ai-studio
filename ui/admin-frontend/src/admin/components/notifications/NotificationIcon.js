import React, { useEffect } from 'react';
import { Badge, IconButton } from '@mui/material';
import NotificationsIcon from '@mui/icons-material/Notifications';
import { useNavigate } from 'react-router-dom';
import { useNotifications } from '../../context/NotificationContext';

const NotificationIcon = () => {
	const navigate = useNavigate();
	const { unreadCount, fetchUnreadCount } = useNotifications();

	useEffect(() => {
		fetchUnreadCount();
		// Poll for new notifications every minute
		const interval = setInterval(fetchUnreadCount, 60000);

		return () => clearInterval(interval);
	}, [fetchUnreadCount]);

	const handleClick = () => {
		navigate('/notifications');
	};

	return (
		<IconButton onClick={handleClick} sx={{ color: 'white' }}>
			<Badge badgeContent={unreadCount} color="error">
				<NotificationsIcon />
			</Badge>
		</IconButton>
	);
};

export default NotificationIcon;
