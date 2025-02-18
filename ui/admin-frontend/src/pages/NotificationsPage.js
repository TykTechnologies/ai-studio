import React from 'react';
import NotificationList from '../admin/components/notifications/NotificationList';
import { Box } from '@mui/material';

const NotificationsPage = () => {
	return (
		<Box sx={{ p: 3 }}>
			<NotificationList />
		</Box>
	);
};

export default NotificationsPage;
