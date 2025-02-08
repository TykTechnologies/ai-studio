import { useEffect, useRef, useState, useCallback } from 'react';
import pubClient from '../../../../admin/utils/pubClient';

export const useChatWebSocket = ({ chatId, onMessageReceived, updateChatName }) => {
	const [isConnected, setIsConnected] = useState(false);
	const [isLoading, setIsLoading] = useState(true);
	const [sessionId, setSessionId] = useState(null);
	const [isNewChat, setIsNewChat] = useState(true);
	const [hasUpdatedChatName, setHasUpdatedChatName] = useState(false);
	const ws = useRef(null);
	const reconnectAttempts = useRef(0);

	const closeWebSocket = useCallback(() => {
		if (ws.current) {
			ws.current.close();
			ws.current = null;
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
			const historicalMessages = response.data
				.map((msg) => {
					try {
						const parsedContent = JSON.parse(msg.attributes.content);

						if (parsedContent.role === "system" || parsedContent.role === "tool") {
							return null;
						}

						if (parsedContent.parts && parsedContent.parts[0]?.type === "tool_call") {
							const toolCall = parsedContent.parts[0].tool_call;
							return {
								type: "ai",
								content: `:::system AI Tool Call: ${toolCall.function.name}:::`,
								isComplete: true,
							};
						}

						let content = parsedContent.text;
						if (parsedContent.role === "human") {
							const messageMatch = content.match(/Message:\s*([\s\S]*)/);
							content = messageMatch ? messageMatch[1].trim() : content;
						}

						return {
							type: parsedContent.role === "human" ? "user" : "ai",
							content: content,
							isComplete: true,
						};
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
				content: ":::system Error: Failed to load chat history:::",
				isComplete: true,
			}];
		}
	}, []);

	useEffect(() => {
		const searchParams = new URLSearchParams(window.location.search);
		const continueId = searchParams.get("continue_id");
		const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";
		const wsUrl = process.env.NODE_ENV === "development"
			? `${wsProtocol}//localhost:8080/common/ws/chat/${chatId}${continueId ? `?session_id=${continueId}` : ""}`
			: `${wsProtocol}//${window.location.host}/common/ws/chat/${chatId}${continueId ? `?session_id=${continueId}` : ""}`;

		setIsNewChat(!continueId);
		setHasUpdatedChatName(false);

		let keepAliveInterval;

		const setupWebSocket = () => {
			closeWebSocket();

			ws.current = new WebSocket(wsUrl);

			ws.current.onopen = () => {
				setIsConnected(true);
				setIsLoading(false);

				keepAliveInterval = setInterval(() => {
					if (ws.current && ws.current.readyState === WebSocket.OPEN) {
						ws.current.send(JSON.stringify({ type: "ping" }));
					}
				}, 10000);

				if (continueId) {
					fetchChatHistory(continueId).then((messages) => {
						if (Array.isArray(messages)) {
							messages.forEach(msg => {
								if (msg.type === 'user') {
									onMessageReceived({
										type: 'user_message',
										payload: msg.content,
										isComplete: true
									});
								} else {
									onMessageReceived({
										type: 'ai_message',
										payload: msg.content,
										isComplete: true
									});
								}
							});
						}
					});
					setSessionId(continueId);
				}
			};

			ws.current.onmessage = (event) => {
				const data = JSON.parse(event.data);
				if (data.type === "session_id") {
					setSessionId(data.payload);
					localStorage.setItem("chatSessionId", data.payload);
					const newUrl = `/chat/${chatId}?continue_id=${data.payload}`;
					window.history.replaceState({}, "", newUrl);
				} else if (data.type === "user_message") {
					onMessageReceived({
						type: "user_message",
						payload: data.payload,
						isComplete: true
					});
				} else {
					onMessageReceived(data);
				}
			};

			ws.current.onerror = (error) => {
				console.error("WebSocket error:", error);
				onMessageReceived({
					type: "system",
					payload: `Error: Failed to connect to chat. ${error.message}`,
				});
				setIsLoading(false);
			};

			ws.current.onclose = (event) => {
				setIsConnected(false);
				if (keepAliveInterval) {
					clearInterval(keepAliveInterval);
				}
				if (!event.wasClean) {
					onMessageReceived({
						type: "system",
						payload: `Error: Connection closed unexpectedly: ${event.reason || "Unknown reason"}`,
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
					payload: "Error: Max reconnection attempts reached. Connection permanently closed. Please refresh the page to try again.",
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

		const timer = setTimeout(() => {
			setupWebSocket();
		}, 1000);

		return () => {
			if (keepAliveInterval) {
				clearInterval(keepAliveInterval);
			}
			clearTimeout(reconnectTimeout);
			clearTimeout(timer);
			closeWebSocket();
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
		closeWebSocket
	};
};
