/**
 * Sends a request to an LLM using a custom REST API with secret authentication
 * 
 * @param secret - The API secret or token
 * @param customUrl - The custom API endpoint URL (defaults to the GPT-4o REST endpoint)
 * @param prompt - The prompt to send to the LLM
 * @param model - The model to use (defaults to 'gpt-4o')
 * @returns The LLM response text
 */
export async function sendRequestToLLM(
    secret: string, 
    customUrl: string = 'https://ai-gateway.tyk.technology/llm/rest/openai-gpt-4o/', 
    prompt: string,
    model: string = 'gpt-4o'
) {
    try {
        // Prepare the request payload
        const payload = {
            model: model,
            messages: [
                { role: 'user', content: prompt }
            ],
            temperature: 0.7,
            max_tokens: 500
        };

        // Make the request
        const response = await fetch(customUrl, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${secret}`
            },
            body: JSON.stringify(payload)
        });

        // Check if the request was successful
        if (!response.ok) {
            const errorText = await response.text().catch(() => 'No response body');
            throw new Error(`API request failed with status ${response.status}: ${errorText}`);
        }

        // Parse the response
        const data = await response.json();
        
        // Extract the response text based on the expected response structure
        if (data.choices && data.choices.length > 0 && data.choices[0].message) {
            return data.choices[0].message.content;
        } else if (data.text) {
            return data.text;
        } else {
            console.log('Unexpected response format:', data);
            return JSON.stringify(data);
        }
    } catch (error) {
        console.error('Error sending request to LLM:', error);
        throw error;
    }
}