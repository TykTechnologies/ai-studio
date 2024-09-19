const vendorData = {
  openai: {
    name: "OpenAI",
    logo: "https://upload.wikimedia.org/wikipedia/commons/thumb/1/13/ChatGPT-Logo.png/320px-ChatGPT-Logo.png",
  },
  google_ai: {
    name: "Google AI",
    logo: "https://lh3.googleusercontent.com/RIR1USuPhQgIwCbC6X09bUiRZKCfu5EkZymDuG0mVQpCM42j0y4tvjSFmtZmezPgcfaCxbGSIkCjNlzXSo_p8KVoDqZvS5nEPKoqog",
  },
  anthropic: {
    name: "Anthropic",
    logo: "https://www.anthropic.com/images/icons/safari-pinned-tab.svg",
  },
  vertex: {
    name: "Vertex AI",
    logo: "https://upload.wikimedia.org/wikipedia/commons/thumb/0/05/Vertex_AI_Logo.svg/24px-Vertex_AI_Logo.svg.png",
  },
  huggingface: {
    name: "HuggingFace",
    logo: "https://cdn-lfs.huggingface.co/repos/96/a2/96a2c8468c1546e660ac2609e49404b8588fcf5a748761fa72c154b2836b4c83/942cad1ccda905ac5a659dfd2d78b344fccfb84a8a3ac3721e08f488205638a0?response-content-disposition=inline%3B+filename*%3DUTF-8%27%27hf-logo.svg%3B+filename%3D%22hf-logo.svg%22%3B&response-content-type=image%2Fsvg%2Bxml&Expires=1726963147&Policy=eyJTdGF0ZW1lbnQiOlt7IkNvbmRpdGlvbiI6eyJEYXRlTGVzc1RoYW4iOnsiQVdTOkVwb2NoVGltZSI6MTcyNjk2MzE0N319LCJSZXNvdXJjZSI6Imh0dHBzOi8vY2RuLWxmcy5odWdnaW5nZmFjZS5jby9yZXBvcy85Ni9hMi85NmEyYzg0NjhjMTU0NmU2NjBhYzI2MDllNDk0MDRiODU4OGZjZjVhNzQ4NzYxZmE3MmMxNTRiMjgzNmI0YzgzLzk0MmNhZDFjY2RhOTA1YWM1YTY1OWRmZDJkNzhiMzQ0ZmNjZmI4NGE4YTNhYzM3MjFlMDhmNDg4MjA1NjM4YTA%7EcmVzcG9uc2UtY29udGVudC1kaXNwb3NpdGlvbj0qJnJlc3BvbnNlLWNvbnRlbnQtdHlwZT0qIn1dfQ__&Signature=nOQNTicSiLmDW0ZripsS52ywjEg6HloaQwnUth3BTSfZ0qbA9FOm-Q-sU-FlHjBwOGaXFw9UexUgmbungrP6MqxZPfYc4A32Sm2AYDf7K0Y5KVhhc6f0ER5QmCvWmjNDCidRVjKYN6EHBUvFemLeAkKt0Q5gBZzfvNHMatS5uie0P9Dbou6ze1Mb5Nav6lqGgOmHb7EmCxSYggcsYnKsTdl5i32eeBvFjENr7rmJIWenYuPQSreDpo5iWIg3%7E569-69XYzS2-Uh8FkcEeVMjzd2JCNdrLQBehatpNe1XiH7IxEFgnt1wrKsajOcnxpDz5F%7EV8%7ENU7rgg5PwRV1uW3g__&Key-Pair-Id=K3ESJI6DHPFC7",
  },
  ollama: {
    name: "Ollama",
    logo: "https://ollama.com/public/ollama.png",
  },

  // Add more vendors as needed
};

export const getVendorName = (vendorCode) =>
  vendorData[vendorCode]?.name || vendorCode;
export const getVendorLogo = (vendorCode) =>
  vendorData[vendorCode]?.logo || null;
export const getVendorCodes = () => Object.keys(vendorData);
