import { useEffect, useRef, useState, useCallback } from 'react';
import pubClient from '../../../../admin/utils/pubClient';

export const useChatWebSocket = ({ chatId, onMessageReceived, updateChatName }) => {
	const [isConnected, setIsConnected] = useState(false);
	const [isLoading, setIsLoading] = useState(false);
	const [sessionId, setSessionId] = useState(null);
	const [isNewChat, setIsNewChat] = useState(true);
	const [hasUpdatedChatName, setHasUpdatedChatName] = useState(false);
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
			setIsNewChat(true);
			setHasUpdatedChatName(false);
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
						const parsedContent = JSON.parse(content);

						// Handle different message roles
						const messageContent = parsedContent.context
							? `[CONTEXT]${parsedContent.context}[/CONTEXT]${parsedContent.text}`
							: parsedContent.text;

						switch (parsedContent.role) {
							case 'human':
								console.log('Processing human message:', parsedContent);
								return {
									type: 'user',
									content: messageContent,
									isComplete: true
								};
							case 'ai':
								return {
									type: 'ai',
									content: messageContent,
									isComplete: true
								};
							case 'system':
								const systemText = parsedContent.text.includes(':::system')
									? messageContent
									: `:::system ${messageContent}:::`;
								return {
									type: 'system',
									content: systemText,
									isComplete: true
								};
							default:
								console.log('Unknown role:', parsedContent.role);
								return null;
						}
					} catch (e) {
						// If parsing fails, treat it as an AI message with direct content
						return {
							type: "ai",
							content: msg.attributes.content,
							isComplete: true,
						};
					}
				})
				.filter((msg) => msg !== null);
			return historicalMessages;
		} catch (error) {
			console.error("Error fetching chat history:", error);
			return [{
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

		// Always treat as new chat unless explicitly continuing
		setIsNewChat(!continueId);
		setHasUpdatedChatName(false);

		let keepAliveInterval;

		const setupWebSocket = () => {
			closeWebSocket();

			ws.current = new WebSocket(wsUrl);

			ws.current.onopen = () => {
				setIsConnected(true);
				isConnectedRef.current = true;
				reconnectAttempts.current = 0;

				if (loadingTimeoutRef.current) {
					clearTimeout(loadingTimeoutRef.current);
					loadingTimeoutRef.current = null;
				}

				keepAliveInterval = setInterval(() => {
					if (ws.current && ws.current.readyState === WebSocket.OPEN) {
						ws.current.send(JSON.stringify({ type: "ping" }));
					}
				}, 10000);

				if (continueId) {
					setSessionId(continueId);
					setIsNewChat(false);
					setIsLoading(true); // Set loading before fetching history
					fetchChatHistory(continueId).then((messages) => {
						if (Array.isArray(messages)) {
							// Send all messages at once in history format
							onMessageReceived({
								type: 'history',
								payload: JSON.stringify(messages.map(msg => ({
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
									type: "system",
									payload: `Datasource '${ds.name}' added to room`,
									isComplete: true,
									datasource: { ...ds, type: 'database', uniqueId }
								});
							});
						}
					} else if (data.type === "user_message") {
						onMessageReceived({
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
						type: "system",
						payload: "Error: Failed to parse message from server",
						isComplete: true
					});
				}
			};

			ws.current.onerror = (error) => {
				console.error("WebSocket error:", error);
				onMessageReceived({
					type: "system",
					payload: `Failed to connect to chat. ${error.message}`,
				});
				setIsLoading(false);
			};

			ws.current.onclose = (event) => {
				setIsConnected(false);
				isConnectedRef.current = false;
				if (keepAliveInterval) {
					clearInterval(keepAliveInterval);
				}
				if (!event.wasClean) {
					onMessageReceived({
						type: "system",
						payload: `Connection closed unexpectedly: ${event.reason || "Unknown reason"}`,
					});
					reconnectWithDelay();
				}
			};
		};

		let reconnectTimeout = null;
		const maxReconnectAttempts = 5;
		const initialReconnectDelay = 500;

		const reconnectWithDelay = () => {
			if (reconnectAttempts.current >= maxReconnectAttempts) {
				console.error("Max reconnection attempts reached. Connection permanently closed.");
				onMessageReceived({
					type: "system",
					payload: "Max reconnection attempts reached. Connection permanently closed. Please refresh the page to try again.",
				});
				return;
			}

			const delay = initialReconnectDelay * Math.pow(2, reconnectAttempts.current);
			console.log(`Attempting to reconnect in ${delay / 1000} seconds... (Attempt ${reconnectAttempts.current + 1})`);

			reconnectTimeout = setTimeout(() => {
				reconnectAttempts.current++;
				console.log("Reconnecting WebSocket...");
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
						type: "system",
						payload: "Connection timeout. Please try again.",
						isComplete: true
					});
				}
			}, 10000);
		}, 1000);

		return () => {
			if (keepAliveInterval) {
				clearInterval(keepAliveInterval);
			}
			if (loadingTimeoutRef.current) {
				clearTimeout(loadingTimeoutRef.current);
			}
			clearTimeout(reconnectTimeout);
			clearTimeout(timer);
			closeWebSocket();
			setIsLoading(false);
		};
	}, [chatId, closeWebSocket, onMessageReceived, fetchChatHistory]);

	return {
		isConnected,
		isLoading,
		sessionId,
		isNewChat,
		hasUpdatedChatName,
		setHasUpdatedChatName,
		setIsNewChat,
		sendMessage,
		closeWebSocket,
		error,
		setError
	};
};
