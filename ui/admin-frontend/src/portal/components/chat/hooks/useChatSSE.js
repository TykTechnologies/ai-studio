import { useEffect, useRef, useState, useCallback } from 'react';
import pubClient from '../../../../admin/utils/pubClient';

export const useChatSSE = ({ chatId, onMessageReceived }) => {
	const [isConnected, setIsConnected] = useState(false);
	const [isLoading, setIsLoading] = useState(false);
	const [sessionId, setSessionId] = useState(null);
	const [error, setError] = useState(null);
	const eventSource = useRef(null);
	const reconnectAttempts = useRef(0);
	const isConnectedRef = useRef(false);
	const loadingTimeoutRef = useRef(null);

	const closeConnection = useCallback(() => {
		if (eventSource.current) {
			eventSource.current.close();
			eventSource.current = null;
			setIsConnected(false);
			setSessionId(null);
		}
	}, []);

	const sendMessage = useCallback(async (message) => {
		console.log("Sending message with sessionId:", sessionId);
		if (!sessionId) {
			console.warn("Cannot send message: sessionId is null");
			return false;
		}

		try {
			await pubClient.post(`/common/chat/${chatId}/messages?session_id=${sessionId}`, message);
			return true;
		} catch (error) {
			console.error('Error sending message:', error);
			return false;
		}
	}, [chatId, sessionId]);

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

		let keepAliveInterval;
		const maxReconnectAttempts = 5;
		const initialReconnectDelay = 500;

		const setupEventSource = () => {
			const baseUrl = pubClient.defaults.baseURL;
			const token = localStorage.getItem('token');
			const params = new URLSearchParams();
			if (continueId) {
				params.append('session_id', continueId);
			}
			if (token) {
				params.append('token', token);
			}
			const url = `${baseUrl}/common/chat/${chatId}${params.toString() ? `?${params.toString()}` : ''}`;
			console.log('Setting up SSE connection to:', url);

			if (!eventSource.current || eventSource.current.readyState === EventSource.CLOSED) {
				console.log('Creating new EventSource');
				eventSource.current = new EventSource(url, {
					withCredentials: true
				});
			}

			eventSource.current.onopen = () => {
				console.log('SSE connection established');
				setIsConnected(true);
				isConnectedRef.current = true;
				reconnectAttempts.current = 0;
				setError(null);
				setIsLoading(false);

				if (loadingTimeoutRef.current) {
					clearTimeout(loadingTimeoutRef.current);
					loadingTimeoutRef.current = null;
				}
			};

			// Listen specifically for session_id events
			eventSource.current.addEventListener('session_id', (event) => {
				try {
					console.log('SSE session_id event received:', event.data);
					if (!event.data) {
						return;
					}
					const data = JSON.parse(event.data);
					console.log('Processing session_id message:', data);
					const newSessionId = data.payload;
					console.log('Setting new sessionId:', newSessionId);
					setSessionId(newSessionId);
					// Update URL with new session ID
					const newUrl = `/chat/${chatId}?continue_id=${newSessionId}`;
					console.log('Updating URL to:', newUrl);
					try {
						window.history.replaceState({}, "", newUrl);
						console.log('URL updated successfully');
					} catch (err) {
						console.error('Failed to update URL:', err);
					}

					// Handle tools and datasources
					if (data.tools && Array.isArray(data.tools)) {
						data.tools.forEach(tool => {
							const uniqueId = `tool-${tool.id}`;
							tool.type = 'tool';
							tool.uniqueId = uniqueId;
						});
					}
					if (data.datasources && Array.isArray(data.datasources)) {
						data.datasources.forEach(ds => {
							const uniqueId = `database-${ds.id}`;
							ds.type = 'database';
							ds.uniqueId = uniqueId;
						});
					}
					// Forward the entire session_id message
					onMessageReceived(data);

					if (continueId) {
						setIsLoading(true);
						fetchChatHistory(continueId).then((messages) => {
							if (Array.isArray(messages)) {
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
					}
				} catch (error) {
					console.error("Error parsing SSE message:", error);
					onMessageReceived({
						id: `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
						type: "system",
						payload: "Error: Failed to parse message from server",
						isComplete: true
					});
				}
			});

			// Handle stream_chunk events
			eventSource.current.addEventListener('stream_chunk', (event) => {
				try {
					console.log('SSE stream_chunk received:', event.data);
					if (!event.data) {
						return;
					}
					onMessageReceived({
						type: 'stream_chunk',
						payload: event.data
					});
				} catch (error) {
					console.error("Error handling stream chunk:", error);
				}
			});

			// Handle message events
			eventSource.current.addEventListener('message', (event) => {
				try {
					console.log('SSE message received:', event.data);
					if (!event.data) {
						return;
					}
					const data = JSON.parse(event.data);
					onMessageReceived(data);
				} catch (error) {
					console.error("Error parsing message:", error);
				}
			});

			// Handle system events
			eventSource.current.addEventListener('system', (event) => {
				try {
					console.log('SSE system message received:', event.data);
					if (!event.data) {
						return;
					}
					const messageContent = event.data.includes(':::system')
						? event.data
						: `:::system ${event.data}:::`;
					onMessageReceived({
						id: `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
						type: 'system',
						content: messageContent,
						isComplete: true
					});
				} catch (error) {
					console.error("Error handling system message:", error);
				}
			});

			// Helper function to detect error type
			const detectErrorType = (error) => {
				if (!error) return 'connection';
				const errorStr = error.toString().toLowerCase();

				if (errorStr.includes('failed to create message') ||
					errorStr.includes('llm') ||
					errorStr.includes('model') ||
					errorStr.includes('anthropic') ||
					errorStr.includes('openai')) {
					return 'llm_config';
				}

				if (errorStr.includes('connection') ||
					errorStr.includes('network') ||
					errorStr.includes('timeout')) {
					return 'connection';
				}

				return 'other';
			};

			// Handle error events
			eventSource.current.addEventListener('error', (event) => {
				try {
					console.log('SSE error message received:', event.data);
					if (!event.data) {
						return;
					}
					const errorType = detectErrorType(event.data);

					// For LLM config errors, don't attempt reconnection
					if (errorType === 'llm_config') {
						reconnectAttempts.current = maxReconnectAttempts; // Prevent reconnection
					}

					onMessageReceived({
						id: `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
						type: 'system',
						content: `:::system Error: ${event.data}:::`,
						errorType: errorType,
						isComplete: true
					});
				} catch (error) {
					console.error("Error handling error message:", error);
				}
			});

			// Handle any other events
			eventSource.current.onmessage = (event) => {
				try {
					console.log('SSE generic message received:', event.data);
					if (!event.data) {
						return;
					}
					const data = JSON.parse(event.data);
					onMessageReceived(data);
				} catch (error) {
					console.error("Error parsing generic message:", error);
					onMessageReceived({
						id: `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
						type: "system",
						payload: "Error: Failed to parse message from server",
						isComplete: true
					});
				}
			};

			eventSource.current.onerror = (error) => {
				console.error("SSE error:", error);
				setIsConnected(false);
				isConnectedRef.current = false;
				setIsLoading(false);

				// Check if we have a recent LLM config error
				const hasLLMError = error?.data && detectErrorType(error.data) === 'llm_config';

				if (!hasLLMError && reconnectAttempts.current < maxReconnectAttempts) {
					onMessageReceived({
						id: `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
						type: "system",
						payload: `Connection lost. Attempting to reconnect... (Attempt ${reconnectAttempts.current + 1}/${maxReconnectAttempts})`,
						errorType: 'connection'
					});

					const delay = initialReconnectDelay * Math.pow(2, reconnectAttempts.current);
					console.log(`Attempting to reconnect in ${delay / 1000} seconds... (Attempt ${reconnectAttempts.current + 1}/${maxReconnectAttempts})`);

					setTimeout(() => {
						reconnectAttempts.current++;
						console.log("Reconnecting SSE...");

						if (eventSource.current) {
							eventSource.current.close();
						}

						// Only update URL if it's a connection error
						if (!hasLLMError) {
							setupEventSource();
						}
					}, delay);
				} else {
					const message = hasLLMError
						? "LLM configuration error. Please check your settings."
						: "Maximum reconnection attempts reached. Please refresh the page.";

					onMessageReceived({
						id: `temp_${Math.floor(Math.random() * 1_000_000_000)}`,
						type: "system",
						payload: message,
						errorType: hasLLMError ? 'llm_config' : 'connection'
					});
					setError(message);
				}
			};

			// Handle ping events to keep connection alive
			eventSource.current.addEventListener('ping', () => {
				console.log('Received ping');
			});
		};

		setIsLoading(true);
		const timer = setTimeout(() => {
			setupEventSource();

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
			clearTimeout(timer);
			closeConnection();
			setIsLoading(false);
			setIsConnected(false);
			isConnectedRef.current = false;
		};
	}, [chatId, closeConnection, onMessageReceived, fetchChatHistory]);

	return {
		isConnected,
		isLoading,
		sessionId,
		sendMessage,
		closeConnection,
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
					byteCount = contentString.length;
				}

				// Build the new system message
				const systemMsg = contentString && contentString.trim() 
					? `\n:::systemUsing function: \`${functionName}()\`::::::systemParameters: ${JSON.stringify(parameters)}::::::systemContent: Function \`${functionName}()\` returned: \`${byteCount}\` bytes:::\n
[CONTEXT]${contentString}[/CONTEXT]\n`
					: `\n:::systemUsing function: \`${functionName}()\`::::::systemParameters: ${JSON.stringify(parameters)}::::::systemContent: Function \`${functionName}()\` returned: \`${byteCount}\` bytes:::\n`;

				// Append the system block to the current AI explanation.
				current.content += systemMsg;

				// Remove the tool_use and tool_result messages.
				result.splice(i + 1, 2);
			}
		}
	}

	return result;
};
