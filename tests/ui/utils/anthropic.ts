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

        // Return the response content
        return response.content.map(item => item.text).join('');
    } catch (error) {
        console.error('Error sending request to Anthropic LLM:', error);
        throw error;
    }
}