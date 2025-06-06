import Anthropic from '@anthropic-ai/sdk';

export async function sendRequestToAnthropicLLMWithSDK(
    secret: string, 
    customUrl: string, 
    prompt: string,
    model: string = 'claude-3-opus-20240229',
    maxTokens: number = 1000
) {
    try {
        // Initialize the Anthropic client
        const anthropic = new Anthropic({
            apiKey: secret,
            baseURL: customUrl
        });

        // Make the request
        const response = await anthropic.messages.create({
            model: model,
            messages: [
                { role: 'user', content: prompt }
            ],
            max_tokens: maxTokens
        });

        // Log the response for debugging
        console.log('Response structure:', JSON.stringify(response, null, 2));
        
        // Handle different response structures
        if (response.content && Array.isArray(response.content)) {
            return response.content.map(item => item.text).join('');
        } else if (response.content && response.content.text) {
            return response.content.text;
        } else if (typeof response.content === 'string') {
            return response.content;
        } else {
            console.error('Unexpected response structure:', response);
            return JSON.stringify(response);
        }
    } catch (error) {
        console.error('Error sending request to Anthropic LLM:', error);
        throw error;
    }
}