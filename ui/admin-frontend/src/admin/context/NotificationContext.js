import React, { createContext, useContext, useState, useCallback } from 'react';
import axios from 'axios';

const NotificationContext = createContext();

export const NotificationProvider = ({ children }) => {
	const [unreadCount, setUnreadCount] = useState(0);

	const fetchUnreadCount = useCallback(async () => {
		try {
			const response = await axios.get('/common/api/v1/notifications/unread/count');
			setUnreadCount(response.data.count);
		} catch (error) {
			console.error('Error fetching unread notifications:', error);
		}
	}, []);

	const markAsRead = useCallback(async (id) => {
		try {
			await axios.put(`/common/api/v1/notifications/${id}/read`);
			// Update the unread count
			fetchUnreadCount();
			return true;
		} catch (error) {
			console.error('Error marking notification as read:', error);
			return false;
		}
	}, [fetchUnreadCount]);

	return (
		<NotificationContext.Provider value={{ unreadCount, fetchUnreadCount, markAsRead }}>
			{children}
		</NotificationContext.Provider>
	);
};

export const useNotifications = () => {
	const context = useContext(NotificationContext);
	if (!context) {
		throw new Error('useNotifications must be used within a NotificationProvider');
	}
	return context;
};
