import logging

from sentence_transformers import SentenceTransformer
from flask import Flask, request, jsonify
import torch
from transformers import pipeline


app = Flask(__name__)

# Set up basic logging configuration
logging.basicConfig(level=logging.INFO,
                    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
                    datefmt='%Y-%m-%d %H:%M:%S')

logger = logging.getLogger(__name__)

# /v1/summarize endpoint summarizes long text
@app.route('/v1/summarize', methods=['POST'])
def summarize():
    try:
        data = request.get_json()
        if not data or 'input' not in data:
            raise ValueError("Missing 'input' in request data")

        hf_name = 'pszemraj/led-large-book-summary'
        wall_of_text = data["input"]

        if not wall_of_text:
            raise ValueError("Empty input text")

        logger.info(f"Received summarization request with {len(wall_of_text)} characters")
    except Exception as e:
        logger.error(f"Error in summarize endpoint: {str(e)}")
        return jsonify({"error": str(e)}), 400

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
        try:
            data = request.get_json()
            if not data or 'input' not in data or 'model' not in data:
                raise ValueError("Missing 'input' or 'model' in request data")

            input_text = data['input']
            model_name = data['model']

            if not input_text or not model_name:
                raise ValueError("Empty input text or model name")

            logger.info(f"Received embedding request for model {model_name} with {len(input_text)} characters")
        except Exception as e:
            logger.error(f"Error in embeddings endpoint: {str(e)}")
            return jsonify({"error": str(e)}), 400
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
