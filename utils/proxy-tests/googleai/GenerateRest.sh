#!/bin/bash

echo "http://localhost:9090/llm/rest/gemini/v1beta/models/gemini-1.5-flash:generateContent?key=$MIDSOMMAR_GOOGLE_KEY"

curl "http://localhost:9090/llm/rest/gemini/v1beta/models/gemini-1.5-flash:generateContent?key=$MIDSOMMAR_GOOGLE_KEY" \
    -H 'Content-Type: application/json' \
    -X POST \
    -d '{
      "contents": [{
        "parts":[{"text": "Write a story about a magic backpack."}]
        }]
       }' 2> /dev/null
