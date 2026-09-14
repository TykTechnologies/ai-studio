import React, { useEffect, useState } from 'react';
import {
	List,
	ListItemButton,
	ListItemIcon,
	ListItemText,
	ListSubheader,
	Typography,
	Paper,
	Container,
	IconButton,
	Box,
	Button,
	FormControlLabel,
	Switch,
	CircularProgress,
	Tooltip,
} from '@mui/material';
import DoneIcon from '@mui/icons-material/Done';
import DoneAllIcon from '@mui/icons-material/DoneAll';
import { useNavigate } from 'react-router-dom';
import { useNotifications } from '../../context/NotificationContext';
import {
	NotificationTypeIcon,
	getPreview,
	groupByDay,
	relativeTime,
} from './notificationPresentation';
import { openNotificationLink } from './NotificationPanel';

/**
 * The full /notifications page: every notification grouped by day, with an
 * unread-only toggle, per-item and bulk mark-as-read, and offset paging
 * through "Load more". Data comes from the NotificationProvider.
 */
const NotificationList = () => {
	const navigate = useNavigate();
	const {
		items,
		total,
		hasMore,
		loading,
		error,
		unreadCount,
		refresh,
		loadMore,
		markAsRead,
		markAllAsRead,
	} = useNotifications();
	const [unreadOnly, setUnreadOnly] = useState(false);

	useEffect(() => {
		refresh({ unread: unreadOnly });
	}, [refresh, unreadOnly]);

	const handleNotificationClick = async (notification) => {
		if (!notification.read) {
			await markAsRead(notification.id);
		}
		openNotificationLink(notification.link, navigate);
	};

	// With the unread filter on, a read item no longer belongs in the list;
	// refetch rather than leaving a stale row.
	const handleMarkAsRead = async (id) => {
		const ok = await markAsRead(id);
		if (ok && unreadOnly) refresh({ unread: true });
	};

	const handleMarkAllAsRead = async () => {
		const ok = await markAllAsRead();
		if (ok && unreadOnly) refresh({ unread: true });
	};

	const groups = groupByDay(items);
	const now = new Date();
	const hasUnread = items.some((notification) => !notification.read);

	return (
		<Container maxWidth="md" sx={{ mt: 4 }}>
			<Paper>
				<Box
					sx={{
						p: 2,
						display: 'flex',
						justifyContent: 'space-between',
						alignItems: 'center',
						flexWrap: 'wrap',
						gap: 1,
					}}
				>
					<Typography variant="headingXLarge" component="h1">
						Notifications
					</Typography>
					<Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
						<FormControlLabel
							control={
								<Switch
									size="small"
									checked={unreadOnly}
									onChange={(event) => setUnreadOnly(event.target.checked)}
								/>
							}
							label={unreadCount > 0 ? `Unread only (${unreadCount})` : 'Unread only'}
						/>
						<Button
							variant="outlined"
							size="small"
							startIcon={<DoneAllIcon />}
							onClick={handleMarkAllAsRead}
							disabled={!hasUnread && unreadCount === 0}
						>
							Mark all as read
						</Button>
					</Box>
				</Box>
				{error && (
					<Typography variant="bodyMediumDefault" color="error" sx={{ px: 2, pb: 1 }}>
						Notifications could not be loaded.
					</Typography>
				)}
				{items.length === 0 && !loading ? (
					<Box sx={{ px: 2, py: 6, textAlign: 'center' }} data-testid="notifications-empty">
						<Typography variant="headingMedium" component="p">
							You're all caught up.
						</Typography>
						<Typography variant="bodyMediumDefault" color="text.defaultSubdued">
							{unreadOnly ? 'There are no unread notifications.' : 'New notifications will show up here.'}
						</Typography>
					</Box>
				) : (
					<List disablePadding>
						{groups.map((group) => (
							<li key={group.label}>
								<ul style={{ padding: 0 }}>
									<ListSubheader disableSticky sx={{ lineHeight: '36px' }}>
										{group.label}
									</ListSubheader>
									{group.items.map((notification) => (
										<ListItemButton
											key={notification.id}
											onClick={() => handleNotificationClick(notification)}
											alignItems="flex-start"
											sx={{
												backgroundColor: notification.read ? 'transparent' : 'rgba(35, 226, 194, 0.08)',
											}}
											data-testid={`notification-row-${notification.id}`}
										>
											<ListItemIcon sx={{ minWidth: 40, mt: 0.5 }}>
												<NotificationTypeIcon type={notification.type} />
											</ListItemIcon>
											<ListItemText
												primary={
													<Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
														<Typography
															variant={notification.read ? 'bodyMediumDefault' : 'bodyMediumSemiBold'}
															sx={{ flexGrow: 1 }}
														>
															{notification.title}
														</Typography>
														<Typography variant="bodySmallDefault" color="text.defaultSubdued">
															{relativeTime(notification.sentAt || notification.createdAt, now)}
														</Typography>
													</Box>
												}
												secondary={getPreview(notification.content)}
												secondaryTypographyProps={{ title: notification.content || '' }}
											/>
											{!notification.read && (
												<Tooltip title="Mark as read">
													<IconButton
														edge="end"
														aria-label="mark as read"
														size="small"
														sx={{ ml: 1, mt: 0.5 }}
														onClick={(e) => {
															e.stopPropagation();
															handleMarkAsRead(notification.id);
														}}
													>
														<DoneIcon fontSize="small" />
													</IconButton>
												</Tooltip>
											)}
										</ListItemButton>
									))}
								</ul>
							</li>
						))}
					</List>
				)}
				<Box sx={{ p: 2, display: 'flex', justifyContent: 'center', alignItems: 'center', gap: 2 }}>
					{loading && <CircularProgress size={20} aria-label="Loading notifications" />}
					{hasMore && !loading && (
						<Button variant="text" onClick={() => loadMore()}>
							Load more ({total - items.length} remaining)
						</Button>
					)}
				</Box>
			</Paper>
		</Container>
	);
};

export default NotificationList;
