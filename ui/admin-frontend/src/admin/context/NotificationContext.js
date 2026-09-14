import React, { createContext, useContext, useState, useCallback, useEffect, useRef, useMemo } from 'react';
import {
	listNotifications,
	getUnreadCount,
	markNotificationRead,
	markAllNotificationsRead,
	PAGE_SIZE,
} from '../services/notificationService';

const NotificationContext = createContext();

/** How often the unread badge is refreshed while the app is open. */
export const UNREAD_POLL_INTERVAL_MS = 60000;

/**
 * NotificationProvider holds the unread count for the bell badge and the
 * latest page of notifications for the panel and the /notifications page.
 * One poll refreshes the count every minute; the list is (re)fetched on
 * demand by refresh() and grown by loadMore().
 */
export const NotificationProvider = ({ children }) => {
	const [unreadCount, setUnreadCount] = useState(0);
	const [items, setItems] = useState([]);
	const [total, setTotal] = useState(0);
	const [loading, setLoading] = useState(false);
	const [error, setError] = useState(null);
	// Filter the current list was fetched with, so loadMore continues it.
	const [unreadOnly, setUnreadOnly] = useState(false);
	const mounted = useRef(true);

	useEffect(() => {
		mounted.current = true;
		return () => {
			mounted.current = false;
		};
	}, []);

	const fetchUnreadCount = useCallback(async () => {
		try {
			const count = await getUnreadCount();
			if (mounted.current) setUnreadCount(count);
		} catch (error) {
			console.error('Error fetching unread notifications:', error);
		}
	}, []);

	// One poll for the whole app (it used to live in the bell icon, which
	// meant no polling while the icon was unmounted and two polls if it was
	// rendered twice).
	useEffect(() => {
		fetchUnreadCount();
		const interval = setInterval(fetchUnreadCount, UNREAD_POLL_INTERVAL_MS);
		return () => clearInterval(interval);
	}, [fetchUnreadCount]);

	/**
	 * Fetches the first page. `options.unread` narrows the list to unread
	 * notifications; `options.limit` overrides the page size (the panel asks
	 * for fewer than the page).
	 */
	const refresh = useCallback(async (options = {}) => {
		const unread = Boolean(options.unread);
		const limit = options.limit || PAGE_SIZE;
		setLoading(true);
		setError(null);
		try {
			const page = await listNotifications({ limit, offset: 0, unread });
			if (!mounted.current) return page;
			setItems(page.items);
			setTotal(page.total);
			setUnreadOnly(unread);
			if (typeof page.unread === 'number') setUnreadCount(page.unread);
			return page;
		} catch (err) {
			console.error('Error fetching notifications:', err);
			if (mounted.current) setError(err);
			return null;
		} finally {
			if (mounted.current) setLoading(false);
		}
	}, []);

	/** Appends the next page of the current list. */
	const loadMore = useCallback(async () => {
		setLoading(true);
		setError(null);
		try {
			const page = await listNotifications({ limit: PAGE_SIZE, offset: items.length, unread: unreadOnly });
			if (!mounted.current) return page;
			setItems((current) => {
				const seen = new Set(current.map((n) => n.id));
				return [...current, ...page.items.filter((n) => !seen.has(n.id))];
			});
			setTotal(page.total);
			if (typeof page.unread === 'number') setUnreadCount(page.unread);
			return page;
		} catch (err) {
			console.error('Error loading more notifications:', err);
			if (mounted.current) setError(err);
			return null;
		} finally {
			if (mounted.current) setLoading(false);
		}
	}, [items.length, unreadOnly]);

	const markAsRead = useCallback(async (id) => {
		const target = items.find((n) => n.id === id);
		if (target && target.read) return true;
		try {
			await markNotificationRead(id);
			if (mounted.current) {
				setItems((current) => current.map((n) => (n.id === id ? { ...n, read: true } : n)));
				setUnreadCount((count) => Math.max(0, count - 1));
			}
			return true;
		} catch (error) {
			console.error('Error marking notification as read:', error);
			return false;
		}
	}, [items]);

	const markAllAsRead = useCallback(async () => {
		try {
			await markAllNotificationsRead();
			if (mounted.current) {
				setItems((current) => current.map((n) => (n.read ? n : { ...n, read: true })));
				setUnreadCount(0);
			}
			return true;
		} catch (error) {
			console.error('Error marking all notifications as read:', error);
			return false;
		}
	}, []);

	const value = useMemo(
		() => ({
			unreadCount,
			items,
			total,
			hasMore: items.length < total,
			loading,
			error,
			unreadOnly,
			fetchUnreadCount,
			refresh,
			loadMore,
			markAsRead,
			markAllAsRead,
		}),
		[unreadCount, items, total, loading, error, unreadOnly, fetchUnreadCount, refresh, loadMore, markAsRead, markAllAsRead],
	);

	return (
		<NotificationContext.Provider value={value}>
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
