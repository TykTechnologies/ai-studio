# Midsommar - AKA Tyk AI Portal

Tyk AI Portal makes it easy to embrace AI in the organisation.

Tyk AI Portal provides two capabilities laser-focussed on corporate AI deployment:
1. Enable non-technical users to utilise AI in a safe and measure way
2. Enable technical users to deploy AI in a secure and scalable way

For non-technical users, Tyk AI Portal:
- Provides an intuitive chat window to interact with any supported AI Vendor with your personalised system prompt
- Enables the use of internal and external tools for those models in the chat window (e.g. JIRA, Hubspot, etc.)
- Enables the use of internal vector data sources provided by the admin team as part of theiur converations
- Enables th user to upload documents for the AI to use with their prompt

For Technical users:
- Provides an AI Gateway that they can interact with most popular model vendors using native tooling
- Provides secure access provisioning through the AI Portal
- Provides an easy way to browse and decide on which Vendors to interact with
- Provides a universal API for developers to interact with vector data sources

For Administratorts, IT, and Platform Teams:
- Easy to set-up cost monitoring of AI models
- Easy to set up, group-based access to AIs, Tools, and Data sets
- Usage monitoring of Tools, AI models, and developer AI apps
- Simple, scriptable content filter policy enablement for interactions with AI models to protect data security
- Access control and access provisioning for developers in the AI Portal.

## Developer Guide

### Prerequisites
1. Clone this repository
2. Clone the llangchain-go fork at https://github.com/lonelycode/langchaingo (yes, I know, but they have not merged my fixes yet).
3. Get some AI access credentials from any supported vendor

## Getting Started
The App has two sections: the back-end and the UI. The back-end is written in Go and the UI is written in React.

All configuration is in the .env file in the root of the project, there is a smaple provided in the root of the project.

To run the go server:

```bash
cd midsommar
cp .env.example .env
go build
./midsommar
```

To run the Front end:

```bash
cd midsommar/ui/admin-frontend/src
npm start
```

The UI is on `http://localhost:3000`, the proxy is on `http://localhost:9090`, and the API is on `http://localhost:8080/`.

When you open the site up for the first time, register a new account - this account will automatically be made admin and a default user group will be created.

## Structure

The back end is very straightforward, there are three levels:
1. The model layer - this contains nearly all data structures and database-level CRUD operations
2. The service layer - This contains all data access and business logic to the model layer
3. The API layer - This is an interface to the service layer via a REST API.

The front-end is split into two sections: admin and portal, each have their own layouts and components. The admin section is for managing the AI models, tools, and data sources,  and the portal is for interacting with the AI models.

## Building a Final Binary

```
cd midsommar/ui/admin-frontend/src
npm run build
cd ../../..
go build
```

That will process all the static files and embed them into the binary. The binary will now be a full server that serves the UI and the API from the same port.ß
