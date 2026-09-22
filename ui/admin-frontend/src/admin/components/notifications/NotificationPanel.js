import React, { useEffect } from 'react';
import {
	Box,
	Button,
	CircularProgress,
	Divider,
	List,
	ListItemButton,
	ListItemIcon,
	ListItemText,
	Typography,
} from '@mui/material';
import DoneAllIcon from '@mui/icons-material/DoneAll';
import { useNavigate } from 'react-router-dom';
import { useNotifications } from '../../context/NotificationContext';
import {
	NotificationTypeIcon,
	getPreview,
	isExternalLink,
	relativeTime,
} from './notificationPresentation';

/** How many notifications the bell panel shows before "View all". */
export const PANEL_SIZE = 8;

/**
 * Opens a notification's link: external URLs in a new tab, in-app paths via
 * the router. When the link only changes the hash of the page already shown
 * (a plugin page's sub-route, e.g. ".../asset-catalog/requests#req_1"),
 * react-router updates the location but a plugin web component only re-reads
 * its hash on popstate, so that is fired too.
 */
export const openNotificationLink = (link, navigate) => {
	if (!link) return;
	if (isExternalLink(link)) {
		window.open(link, '_blank', 'noopener');
		return;
	}
	const [beforeHash] = link.split('#');
	const current = `${window.location.pathname}${window.location.search}`;
	navigate(link);
	if (beforeHash === current) {
		window.dispatchEvent(new PopStateEvent('popstate'));
	}
};

/**
 * The dropdown under the bell: the latest few notifications with type icon,
 * title, one-line preview, relative time and an unread dot. Clicking an item
 * marks it read and opens its object. Rendered inside NotificationIcon's
 * popover; `onClose` closes that popover after navigation.
 */
const NotificationPanel = ({ onClose, titleId }) => {
	const navigate = useNavigate();
	const { items, total, unreadCount, loading, error, refresh, markAsRead, markAllAsRead } = useNotifications();

	// Refetch the latest page every time the panel opens.
	useEffect(() => {
		refresh({ limit: PANEL_SIZE });
	}, [refresh]);

	const visible = items.slice(0, PANEL_SIZE);
	const now = new Date();

	const handleItemClick = async (notification) => {
		if (!notification.read) {
			await markAsRead(notification.id);
		}
		if (notification.link) {
			openNotificationLink(notification.link, navigate);
		}
		onClose?.();
	};

	const handleViewAll = () => {
		navigate('/notifications');
		onClose?.();
	};

	return (
		<Box sx={{ width: 380, maxWidth: '90vw' }} data-testid="notification-panel">
			<Box sx={{ px: 2, py: 1.5, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
				<Typography id={titleId} variant="headingMedium" component="h2">
					Notifications
				</Typography>
				{unreadCount > 0 && (
					<Typography variant="bodySmallDefault" color="text.defaultSubdued">
						{unreadCount} unread
					</Typography>
				)}
			</Box>
			<Divider />
			{loading && visible.length === 0 ? (
				<Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
					<CircularProgress size={24} aria-label="Loading notifications" />
				</Box>
			) : visible.length === 0 ? (
				<Box sx={{ px: 2, py: 4, textAlign: 'center' }}>
					<Typography variant="bodyMediumDefault" color="text.defaultSubdued">
						{error ? 'Notifications could not be loaded.' : "You're all caught up."}
					</Typography>
				</Box>
			) : (
				<List dense disablePadding sx={{ maxHeight: 420, overflowY: 'auto' }} aria-label="Latest notifications">
					{visible.map((notification) => (
						<ListItemButton
							key={notification.id}
							onClick={() => handleItemClick(notification)}
							alignItems="flex-start"
							sx={{
								px: 2,
								py: 1.25,
								backgroundColor: notification.read ? 'transparent' : 'rgba(35, 226, 194, 0.08)',
							}}
							data-testid={`notification-item-${notification.id}`}
						>
							<ListItemIcon sx={{ minWidth: 36, mt: 0.5 }}>
								<NotificationTypeIcon type={notification.type} fontSize="small" />
							</ListItemIcon>
							<ListItemText
								primary={
									<Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
										<Typography
											variant="bodyMediumSemiBold"
											sx={{ flexGrow: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
										>
											{notification.title}
										</Typography>
										<Typography variant="bodySmallDefault" color="text.defaultSubdued" sx={{ flexShrink: 0 }}>
											{relativeTime(notification.sentAt || notification.createdAt, now)}
										</Typography>
										{!notification.read && (
											<Box
												component="span"
												role="img"
												aria-label="Unread"
												sx={{ width: 8, height: 8, borderRadius: '50%', bgcolor: 'primary.main', flexShrink: 0 }}
											/>
										)}
									</Box>
								}
								secondary={getPreview(notification.content, 90)}
								secondaryTypographyProps={{
									sx: { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' },
									title: notification.content || '',
								}}
							/>
						</ListItemButton>
					))}
				</List>
			)}
			<Divider />
			<Box sx={{ px: 1, py: 0.5, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
				<Button
					size="small"
					startIcon={<DoneAllIcon />}
					onClick={() => markAllAsRead()}
					disabled={unreadCount === 0}
					aria-label="Mark all as read"
				>
					Mark all as read
				</Button>
				<Button size="small" onClick={handleViewAll}>
					View all{total > 0 ? ` (${total})` : ''}
				</Button>
			</Box>
		</Box>
	);
};

export default NotificationPanel;
