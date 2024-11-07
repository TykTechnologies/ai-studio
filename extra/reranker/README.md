## To run this container:

```
docker run -p 4000:80 -e AUTH_KEY=1234 -v /root/tmp:/root/.cache/huggingface/hub docker.io/lonelycode/reranker:latest
```

## To build this container for a server:

```
docker build --no-cache --platform linux/amd64 -t docker.io/lonelycode/reranker:latest .
```

## To rerank a query set:

```
curl --location 'http://127.0.0.1:4000/rerank' \
--header 'Authorization: 1234' \
--header 'Content-Type: application/json' \
--data '{
  "data": [
    {
      "id": "1",
      "title": "foo",
      "pair": ["what is panda?", "hi"]
    },
    {
      "id": "2",
      "title": "bar",
      "pair": ["what is panda?", "The giant panda (Ailuropoda melanoleuca), sometimes called a panda bear or simply panda, is a bear species endemic to China."]
    },
    {
        "id": "3",
        "title": "baz",
        "pair": ["what is panda?", "The giant panda is a bear species from Asia."]
    }
  ]
}'
```