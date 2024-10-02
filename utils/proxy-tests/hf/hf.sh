#!/bin/bash
URL=http://localhost:9090/llm/rest/huggingface-inference/models/cardiffnlp/twitter-roberta-base-sentiment-latest
curl $URL \
-H "Authorization: Bearer $MIDSOMMAR_API_KEY" \
-H 'Content-Type: application/json' \
-d '{"inputs": "Today is a great day"}'
