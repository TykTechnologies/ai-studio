import { useEffect, useRef, useState, useCallback } from 'react';
import pubClient from '../../../../admin/utils/pubClient';

export const useChatWebSocket = ({ chatId, onMessageReceived }) => {
	const [isConnected, setIsConnected] = useState(false);
	const [isLoading, setIsLoading] = useState(false);
	const [sessionId, setSessionId] = useState(null);
	const [error, setError] = useState(null);
	const ws = useRef(null);
	const reconnectAttempts = useRef(0);
	const isConnectedRef = useRef(false);
	const loadingTimeoutRef = useRef(null);

	const closeWebSocket = useCallback(() => {
		if (ws.current) {
			ws.current.close();
			ws.current = null;
			setIsConnected(false);
			setSessionId(null);
		}
	}, []);

	const sendMessage = useCallback((message) => {
		if (ws.current && ws.current.readyState === WebSocket.OPEN) {
			ws.current.send(JSON.stringify(message));
			return true;
		}
		return false;
	}, []);

	const fetchChatHistory = useCallback(async (currentSessionId) => {
		try {
			const response = await pubClient.get(`/common/sessions/${currentSessionId}/messages?limit=100`);
			if (!response.data || !Array.isArray(response.data)) {
				return [];
			}

			const historicalMessages = response.data
				.map((msg) => {
					try {
						const content = msg.attributes?.content || msg.content;
						const messageId = msg.id;
						if (!messageId) {
							console.error('Message ID missing from server response:', msg);
							return null;
						}
						const parsedContent = JSON.parse(content);

						// Handle different message roles
						const messageContent = parsedContent.context
							? `[CONTEXT]${parsedContent.context}[/CONTEXT]${parsedContent.text}`
							: parsedContent.text;

						switch (parsedContent.role) {
							case 'human':
								return {
									id: messageId,
									type: 'user',
									content: messageContent,
									isComplete: true
								};
							case 'ai':
								return {
									id: messageId,
									type: 'ai',
									content: messageContent,
									isComplete: true
								};
							case 'system':
								const systemText = parsedContent.text.includes(':::system')
									? messageContent
									: `:::system ${messageContent}:::`;
								return {
									id: messageId,
									type: 'system',
									content: systemText,
									isComplete: true
								};
							case 'tool':
								return {
									id: messageId,
									type: 'ai',
									content: messageContent,
									isComplete: true
								};
							default:
								console.log('Unknown role:', parsedContent.role);
								return null;
						}
					} catch (e) {
						// If parsing fails, treat it as an AI message with direct content
						return {
							id: msg.id,
							type: "ai",
							content: msg.attributes.content,
							isComplete: true,
						};
					}
				})
				.filter((msg) => msg !== null);

			// Get tools from userEntitlements
			const userEntitlements = JSON.parse(localStorage.getItem("userEntitlements") || "{}");
			const tools = userEntitlements?.data?.tool_catalogues?.[0]?.attributes?.tools || [];

			const reorderedMessages = reorderAndMergeToolMessages(historicalMessages, tools);

			return reorderedMessages;
		} catch (error) {
			console.error("Error fetching chat history:", error);
			return [{
				id: `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
				type: "system",
				content: "Error: Failed to load chat history",
				isComplete: true,
			}];
		}
	}, []);

	useEffect(() => {
		const searchParams = new URLSearchParams(window.location.search);
		const continueId = searchParams.get("continue_id");
		const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";

		// Only use continueId if it's present in the URL
		// This ensures we don't use any stored session when starting a new chat
		const wsUrl = process.env.NODE_ENV === "development"
			? `${wsProtocol}//localhost:8080/common/ws/chat/${chatId}${continueId ? `?session_id=${continueId}` : ""}`
			: `${wsProtocol}//${window.location.host}/common/ws/chat/${chatId}${continueId ? `?session_id=${continueId}` : ""}`;

		let keepAliveInterval;

		const setupWebSocket = () => {
			// Don't close existing connection if we're reconnecting
			if (!ws.current || ws.current.readyState === WebSocket.CLOSED) {
				ws.current = new WebSocket(wsUrl);
			}

			ws.current.onopen = () => {
				console.log('WebSocket connection established');
				setIsConnected(true);
				isConnectedRef.current = true;
				reconnectAttempts.current = 0;
				setError(null); // Clear any previous errors
				setIsLoading(false); // Ensure loading state is cleared

				if (loadingTimeoutRef.current) {
					clearTimeout(loadingTimeoutRef.current);
					loadingTimeoutRef.current = null;
				}

				// The server will send pings every ~54 seconds
				// Browser will automatically respond with pongs
				if (continueId) {
					setSessionId(continueId);
					setIsLoading(true); // Set loading before fetching history
					fetchChatHistory(continueId).then((messages) => {
						if (Array.isArray(messages)) {
							// Send all messages at once in history format
							onMessageReceived({
								type: 'history',
								payload: JSON.stringify(messages.map(msg => ({
									id: msg.id,
									attributes: {
										content: JSON.stringify({
											role: msg.type === 'user' ? 'human' : msg.type === 'ai' ? 'ai' : 'system',
											text: msg.content
										})
									}
								})))
							});
						}
						setIsLoading(false);
					}).catch(() => {
						setIsLoading(false);
					});
				} else {
					setIsLoading(false);
				}
			};

			ws.current.onmessage = (event) => {
				try {
					const data = JSON.parse(event.data);
					if (data.type === "session_id") {
						const newSessionId = data.payload;
						setSessionId(newSessionId);
						// Update URL with new session ID
						const newUrl = `/chat/${chatId}?continue_id=${newSessionId}`;
						window.history.replaceState({}, "", newUrl);

						// Handle tools and datasources
						if (data.tools && Array.isArray(data.tools)) {
							data.tools.forEach(tool => {
								const uniqueId = `tool-${tool.id}`;
								onMessageReceived({
									id: `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
									type: "system",
									payload: `Tool '${tool.name}' added to room`,
									isComplete: true,
									tool: { ...tool, type: 'tool', uniqueId }
								});
							});
						}
						if (data.datasources && Array.isArray(data.datasources)) {
							data.datasources.forEach(ds => {
								const uniqueId = `database-${ds.id}`;
								onMessageReceived({
									id: `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
									type: "system",
									payload: `Datasource '${ds.name}' added to room`,
									isComplete: true,
									datasource: { ...ds, type: 'database', uniqueId }
								});
							});
						}
					} else if (data.type === "user_message") {
						// For user messages, pass through the server's ID
						onMessageReceived({
							id: data.id,
							type: "user_message",
							payload: data.payload,
							isComplete: true
						});
					} else {
						onMessageReceived(data);
					}
				} catch (error) {
					console.error("Error parsing websocket message:", error);
					onMessageReceived({
						id: `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
						type: "system",
						payload: "Error: Failed to parse message from server",
						isComplete: true
					});
				}
			};

			ws.current.onerror = (error) => {
				console.error("WebSocket error:", error);
				// Only set error if we're not in the process of reconnecting
				if (reconnectAttempts.current === 0) {
					onMessageReceived({
						id: `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
						type: "system",
						payload: `WebSocket error occurred. Attempting to reconnect...`,
					});
					setError(`WebSocket error: ${error.message}`);
				}
			};

			ws.current.onclose = (event) => {
				console.log('WebSocket connection closed', event);

				if (keepAliveInterval) {
					clearInterval(keepAliveInterval);
				}

				// Always update connection state
				setIsConnected(false);
				isConnectedRef.current = false;
				setIsLoading(false);

				if (!event.wasClean) {
					// Only show reconnection message if we haven't reached max attempts
					if (reconnectAttempts.current < maxReconnectAttempts) {
						onMessageReceived({
							id: `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
							type: "system",
							payload: `Connection lost. Attempting to reconnect... (Attempt ${reconnectAttempts.current + 1}/${maxReconnectAttempts})`,
						});
						reconnectWithDelay();
					} else {
						onMessageReceived({
							id: `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
							type: "system",
							payload: "Maximum reconnection attempts reached. Please refresh the page.",
						});
						setError("Maximum reconnection attempts reached. Please refresh the page.");
					}
				}
			};
		};

		let reconnectTimeout = null;
		const maxReconnectAttempts = 5;
		const initialReconnectDelay = 500;

		const reconnectWithDelay = () => {
			if (reconnectAttempts.current >= maxReconnectAttempts) {
				console.error("Max reconnection attempts reached.");
				setError("Maximum reconnection attempts reached. Please refresh the page.");
				return;
			}

			const delay = initialReconnectDelay * Math.pow(2, reconnectAttempts.current);
			console.log(`Attempting to reconnect in ${delay / 1000} seconds... (Attempt ${reconnectAttempts.current + 1}/${maxReconnectAttempts})`);

			reconnectTimeout = setTimeout(() => {
				reconnectAttempts.current++;
				console.log("Reconnecting WebSocket...");

				// Close existing connection if it's still around
				if (ws.current) {
					ws.current.close();
				}

				setupWebSocket();
			}, delay);
		};

		setIsLoading(true);
		const timer = setTimeout(() => {
			setupWebSocket();

			// Set loading timeout
			loadingTimeoutRef.current = setTimeout(() => {
				if (!isConnectedRef.current) {
					setIsLoading(false);
					onMessageReceived({
						id: `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
						type: "system",
						payload: "Connection timeout. Please try again.",
						isComplete: true
					});
				}
			}, 10000);
		}, 1000);

		return () => {
			if (loadingTimeoutRef.current) {
				clearTimeout(loadingTimeoutRef.current);
			}
			clearTimeout(reconnectTimeout);
			clearTimeout(timer);
			closeWebSocket();
			setIsLoading(false);
			setIsConnected(false);
			isConnectedRef.current = false;
		};
	}, [chatId, closeWebSocket, onMessageReceived, fetchChatHistory]);

	return {
		isConnected,
		isLoading,
		sessionId,
		sendMessage,
		closeWebSocket,
		error,
		setError
	};
};

const reorderAndMergeToolMessages = (messages) => {
	console.log("Reordering and merging tool messages");
	const result = [...messages];

	//
	// STEP 1: Reorder so that each group appears in the order:
	//   ai (AI response explanation)
	//   ai (tool_use ...)
	//   ai (tool_result ...)
	//
	for (let i = 0; i < result.length; i++) {
		if (result[i]?.type === 'ai' && result[i]?.content.includes('tool_use')) {
			// Check if we have a trio: tool_use, tool_result, and explanation.
			if (
				i + 2 < result.length &&
				result[i + 1]?.type === 'ai' && result[i + 1]?.content.includes('tool_result') &&
				result[i + 2]?.type === 'ai' &&
				!result[i + 2]?.content.includes('tool_use') &&
				!result[i + 2]?.content.includes('tool_result')
			) {
				// Remove the explanation message from its current position
				// and insert it before the tool_use message.
				const explanation = result.splice(i + 2, 1)[0];
				result.splice(i, 0, explanation);
				i += 2; // Skip the group we just processed.
			}
		}
	}

	//
	// STEP 2: Merge tool_use and tool_result *into* the closest AI response.
	//
	// We now expect groups ordered as:
	//   ai (explanation)
	//   ai (tool_use ...)
	//   ai (tool_result ...)
	//
	for (let i = 0; i < result.length; i++) {
		const current = result[i];
		if (
			current?.type === 'ai' &&
			!current.content.includes('tool_use') &&
			!current.content.includes('tool_result')
		) {
			// Check if the next two messages are tool_use and tool_result.
			if (
				i + 2 < result.length &&
				result[i + 1]?.type === 'ai' && result[i + 1].content.includes('tool_use') &&
				result[i + 2]?.type === 'ai' && result[i + 2].content.includes('tool_result')
			) {
				// --- Process the tool_use message ---
				// Remove the "tool_use" prefix and trim the content.
				const toolUseRaw = result[i + 1].content.replace(/\/?tool_use\s*:?/ig, '').trim();
				let functionName = "unknown";
				let parameters = {};
				try {
					const toolUseData = JSON.parse(toolUseRaw);
					// Extract the function name and parameters.
					functionName = toolUseData?.function?.name || functionName;
					parameters = toolUseData?.function?.arguments || parameters;
				} catch (err) {
					console.error('Error parsing tool_use JSON:', err);
				}

				// --- Process the tool_result message ---
				// Remove the "tool_result" prefix and trim the content.
				const toolResultRaw = result[i + 2].content.replace(/\/?tool_result\s*:?/ig, '').trim();
				let contentData = {};
				try {
					const toolResultData = JSON.parse(toolResultRaw);
					contentData = toolResultData?.content || contentData;
				} catch (err) {
					console.error('Error parsing tool_result JSON:', err);
				}

				// Calculate the byte length of the tool result content (its JSON string).
				const contentString = JSON.stringify(contentData);
				let byteCount = 0;
				try {
					// Use TextEncoder to accurately measure the UTF-8 byte length.
					byteCount = new TextEncoder().encode(contentString).length;
				} catch (err) {
					// Fallback (note: this may be inaccurate for multi-byte characters).
					byteCount = contentString.length;
				}

				// --- Build the new system message ---
				// Note: We build a block that starts with :::system and ends with :::,
				// with the required lines inside.
				const systemMsg = `\n:::systemUsing function: \`${functionName}()\`::::::systemParameters: ${JSON.stringify(parameters)}::::::systemContent: Function \`${functionName}()\` returned: \`${byteCount}\` bytes:::\n
[CONTEXT]${contentString}[/CONTEXT]\n`;

				// Append the system block to the current AI explanation.
				current.content += systemMsg;

				// Remove the tool_use and tool_result messages.
				result.splice(i + 1, 2);
			}
		}
	}

	return result;
};
