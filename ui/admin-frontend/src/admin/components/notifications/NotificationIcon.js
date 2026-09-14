import React, { useId, useState } from 'react';
import { Badge, IconButton, Popover, Tooltip } from '@mui/material';
import NotificationsIcon from '@mui/icons-material/Notifications';
import { useNotifications } from '../../context/NotificationContext';
import NotificationPanel from './NotificationPanel';

/**
 * The bell in the top bar. Shows the unread badge (polled by the
 * NotificationProvider) and opens the NotificationPanel in a popover
 * instead of sending the user to a full page.
 */
const NotificationIcon = ({ sx }) => {
	const { unreadCount } = useNotifications();
	const [anchorEl, setAnchorEl] = useState(null);
	const panelId = useId();
	const titleId = `${panelId}-title`;
	const open = Boolean(anchorEl);

	const handleOpen = (event) => setAnchorEl(event.currentTarget);
	const handleClose = () => setAnchorEl(null);

	const label = unreadCount > 0 ? `Notifications, ${unreadCount} unread` : 'Notifications';

	return (
		<>
			<Tooltip title="Notifications">
				<IconButton
					onClick={handleOpen}
					sx={{ color: 'white', ...sx }}
					aria-label={label}
					aria-haspopup="dialog"
					aria-expanded={open}
					aria-controls={open ? panelId : undefined}
					data-testid="notification-bell"
				>
					<Badge badgeContent={unreadCount} color="error" max={99}>
						<NotificationsIcon />
					</Badge>
				</IconButton>
			</Tooltip>
			<Popover
				id={panelId}
				open={open}
				anchorEl={anchorEl}
				onClose={handleClose}
				anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
				transformOrigin={{ vertical: 'top', horizontal: 'right' }}
				slotProps={{ paper: { role: 'dialog', 'aria-labelledby': titleId, sx: { mt: 1 } } }}
			>
				{open && <NotificationPanel onClose={handleClose} titleId={titleId} />}
			</Popover>
		</>
	);
};

export default NotificationIcon;
