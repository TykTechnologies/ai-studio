import React from 'react';
import NotificationsIcon from '@mui/icons-material/Notifications';
import AppsIcon from '@mui/icons-material/Apps';
import PersonIcon from '@mui/icons-material/Person';
import AssignmentTurnedInIcon from '@mui/icons-material/AssignmentTurnedIn';
import ExtensionIcon from '@mui/icons-material/Extension';
import AccountBalanceWalletIcon from '@mui/icons-material/AccountBalanceWallet';
import VpnKeyIcon from '@mui/icons-material/VpnKey';
import FolderSharedIcon from '@mui/icons-material/FolderShared';
import { format, isToday, isYesterday, isValid } from 'date-fns';

// Presentation helpers shared by the bell panel and the /notifications page:
// the type-to-icon map, relative times, previews and day grouping.

const TYPE_ICONS = {
	app: AppsIcon,
	user: PersonIcon,
	submission: AssignmentTurnedInIcon,
	budget: AccountBalanceWalletIcon,
	access: VpnKeyIcon,
	catalog: FolderSharedIcon,
	catalogue: FolderSharedIcon,
};

/** Icon component for a notification type; plugin:* types share one icon. */
export const iconForType = (type) => {
	const key = String(type || '').toLowerCase();
	if (key.startsWith('plugin')) return ExtensionIcon;
	const base = key.split(/[.:/]/)[0];
	return TYPE_ICONS[base] || NotificationsIcon;
};

export const NotificationTypeIcon = ({ type, ...props }) => {
	const IconComponent = iconForType(type);
	return <IconComponent data-testid={`notification-type-${String(type || 'default').toLowerCase() || 'default'}`} {...props} />;
};

export const toDate = (value) => {
	if (!value) return null;
	const date = value instanceof Date ? value : new Date(value);
	return isValid(date) ? date : null;
};

/** "just now", "5m ago", "3h ago", "2d ago", then a short date. */
export const relativeTime = (value, now = new Date()) => {
	const date = toDate(value);
	if (!date) return '';
	const seconds = Math.round((now.getTime() - date.getTime()) / 1000);
	if (seconds < 45) return 'just now';
	const minutes = Math.round(seconds / 60);
	if (minutes < 60) return `${minutes}m ago`;
	const hours = Math.round(minutes / 60);
	if (hours < 24) return `${hours}h ago`;
	const days = Math.round(hours / 24);
	if (days < 7) return `${days}d ago`;
	return format(date, now.getFullYear() === date.getFullYear() ? 'd MMM' : 'd MMM yyyy');
};

export const PREVIEW_LENGTH = 160;

/** Content is plain text; collapse whitespace and cut it to one line's worth. */
export const getPreview = (content, length = PREVIEW_LENGTH) => {
	const text = (content || '').replace(/\s+/g, ' ').trim();
	if (text.length <= length) return text;
	return `${text.slice(0, length).trimEnd()}…`;
};

export const isExternalLink = (link) => /^https?:\/\//i.test(link || '');

/** Label for the day a notification belongs to. */
export const dayLabel = (value, now = new Date()) => {
	const date = toDate(value);
	if (!date) return 'Earlier';
	if (isToday(date)) return 'Today';
	if (isYesterday(date)) return 'Yesterday';
	return format(date, now.getFullYear() === date.getFullYear() ? 'EEEE, d MMMM' : 'd MMMM yyyy');
};

/**
 * Groups notifications (already newest-first) into [{ label, items }] by day,
 * preserving order. Undated items collect under "Earlier" at the end.
 */
export const groupByDay = (notifications, now = new Date()) => {
	const groups = [];
	const undated = [];
	const index = new Map();
	(notifications || []).forEach((notification) => {
		const date = toDate(notification.sentAt || notification.createdAt);
		if (!date) {
			undated.push(notification);
			return;
		}
		const label = dayLabel(date, now);
		if (!index.has(label)) {
			index.set(label, { label, items: [] });
			groups.push(index.get(label));
		}
		index.get(label).items.push(notification);
	});
	if (undated.length) groups.push({ label: 'Earlier', items: undated });
	return groups;
};
