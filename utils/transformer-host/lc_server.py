from sentence_transformers import SentenceTransformer
from flask import Flask, request, jsonify
import torch
from transformers import pipeline


app = Flask(__name__)

# /v1/summarize endpoint sumamrizes long text
@app.route('/v1/summarize', methods=['POST'])
def summarize():
    data = request.get_json()
    hf_name = 'pszemraj/led-large-book-summary'
    wall_of_text = data["input"]

    summarizer = pipeline(
        "summarization",
        hf_name,
        device=0 if torch.cuda.is_available() else -1,
    )

    result = summarizer(
        wall_of_text,
        min_length=16,
        max_length=256,
        no_repeat_ngram_size=3,
        encoder_no_repeat_ngram_size=3,
        repetition_penalty=3.5,
        num_beams=4,
        early_stopping=True,
    )

    response_data = {
        "summary": result
    }


    return jsonify(response_data)

@app.route('/embeddings', methods=['POST'])
def embeddings():
    data = request.get_json()
    input_text = data['input']
    model_name = data['model']
    model = SentenceTransformer(model_name)
    embeddings = model.encode(input_text).tolist()
    response_data = {
        "data": [
            {
                "embedding": embedding,
                "index": i,
                "object": "embedding"
            } for i, embedding in enumerate(embeddings)
        ],
        "model": model_name,
        "object": "list",
        "usage": {
            "prompt_tokens": len(input_text),
            "total_tokens": len(input_text)
        }
    }
    return jsonify(response_data)

if __name__ == '__main__':
    app.run(port=8000, host="0.0.0.0")
