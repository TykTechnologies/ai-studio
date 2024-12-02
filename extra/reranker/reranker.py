import os
import torch
from transformers import AutoModelForSequenceClassification, AutoTokenizer
from flask import Flask, request, jsonify
from threading import Thread
import time

app = Flask(__name__)

tokenizer = None
model = None
device = None

def load_model_and_tokenizer():
    global tokenizer, model, device
    device = "cuda:0" if torch.cuda.is_available() else "cpu"
    model_name = 'BAAI/bge-reranker-large'
    tokenizer = AutoTokenizer.from_pretrained(model_name)
    model = AutoModelForSequenceClassification.from_pretrained(model_name)
    model = model.to(device)
    model.eval()

# Run the load_model_and_tokenizer function in a background thread to avoid blocking
Thread(target=load_model_and_tokenizer).start()

@app.route('/rerank', methods=['POST'])
def rerank():
    # Check if model and tokenizer are loaded
    if tokenizer is None or model is None:
        return jsonify({'error': 'Model not yet loaded. Try again later.'}), 503

    auth_key = request.headers.get('Authorization')
    # if auth_key != os.getenv('AUTH_KEY'):
    #     abort(401)  # Unauthorized

    data = request.json['data']
    pairs = [item['pair'] for item in data]
    ids = [item['id'] for item in data]
    titles = [item['title'] for item in data]
    with torch.no_grad():
        inputs = tokenizer(pairs, padding=True, truncation=True, return_tensors='pt', max_length=512).to(device)
        scores = model(**inputs, return_dict=True).logits.view(-1, ).float().tolist()
        results = [{'id': id, 'title': title, 'pair': pair, 'score': score} for id, title, pair, score in zip(ids, titles, pairs, scores)]
        results.sort(key=lambda x: x['score'], reverse=True)
    return jsonify(results=results)

if __name__ == '__main__':
    debug_mode = os.getenv('DEBUG_MODE', 'False') == 'True'
    app.run(debug=debug_mode)
