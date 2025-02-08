import React from 'react';
import SystemMessage from './SystemMessage';
import ContextMessage from './ContextMessage';
import MarkdownMessage from './MarkdownMessage';

const MessageContent = ({ content, messageIndex, expandedGroups, toggleGroup }) => {
	if (!content) {
		return null;
	}

	const segments = content.split(/((?::::|\%\%\%)system[\s\S]*?(?::::|\%\%\%)|\[CONTEXT\][\s\S]*?\[\/CONTEXT\])/g);

	const groupSystemMessages = (segments) => {
		let groupedSegments = [];
		let currentSystemGroup = [];

		segments.forEach((segment) => {
			if (segment.match(/(:::|\%\%\%)system/)) {
				currentSystemGroup.push(
					segment
						.replace(/(:::|\%\%\%)system\s*([\s\S]*?)(:::|\%\%\%)/, '$2')
						.trim()
				);
			} else if (segment.match(/\[CONTEXT\][\s\S]*?\[\/CONTEXT\]/)) {
				const contextContent = segment
					.replace(/\[CONTEXT\]([\s\S]*?)\[\/CONTEXT\]/, '$1')
					.trim();
				groupedSegments.push({
					type: 'context-group',
					messages: [contextContent],
				});
			} else if (segment.trim()) {
				if (currentSystemGroup.length > 0) {
					groupedSegments.push({
						type: 'system-group',
						messages: currentSystemGroup,
					});
					currentSystemGroup = [];
				}
				if (segment.trim()) {
					groupedSegments.push({
						type: 'content',
						content: segment,
					});
				}
			}
		});

		if (currentSystemGroup.length > 0) {
			groupedSegments.push({
				type: 'system-group',
				messages: currentSystemGroup,
			});
		}

		return groupedSegments;
	};

	const groupedSegments = groupSystemMessages(segments);

	return (
		<>
			{groupedSegments.map((segment, index) => {
				const groupId = `${segment.type}-${messageIndex}-${index}`;
				const isExpanded = expandedGroups[groupId];

				if (segment.type === 'system-group') {
					return (
						<SystemMessage
							key={groupId}
							messages={segment.messages}
							groupId={groupId}
							isExpanded={isExpanded}
							toggleGroup={toggleGroup}
						/>
					);
				} else if (segment.type === 'context-group') {
					return (
						<ContextMessage
							key={groupId}
							content={segment.messages[0]}
							groupId={groupId}
							isExpanded={isExpanded}
							toggleGroup={toggleGroup}
						/>
					);
				} else {
					return (
						<MarkdownMessage
							key={groupId}
							content={segment.content}
						/>
					);
				}
			})}
		</>
	);
};

export default MessageContent;
