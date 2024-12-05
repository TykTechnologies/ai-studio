---
title: "Configuration"
weight: 2
# bookFlatSection: false
# bookToc: true
# bookHidden: false
# bookCollapseSection: false
# bookComments: false
# bookSearchExclude: false
---

### Configuration

The Tyk AI Portal is configured using environment variables that can be set in a `.env` file. Below are the available configuration options:

---

#### **Database Configuration**

The AI Portal supports two database types:
- SQLite (recommended for testing only)
- PostgreSQL (recommended for production use)

```env
DATABASE_TYPE=postgres
DATABASE_URL=postgresql://user:password@localhost:5432/dbname
```

For SQLite, use:
```env
DATABASE_TYPE=sqlite
DATABASE_URL=midsommar.db
```

---

#### **Email Configuration**

SMTP settings for sending system emails:

```env
SMTP_SERVER=smtp.sendgrid.net
SMTP_PORT=587
SMTP_USER=apikey
SMTP_PASS=your_smtp_password
```

Email-related settings:
```env
FROM_EMAIL=noreply@tyk.io
ADMIN_EMAIL=you@tyk.io
```

---

#### **Registration Settings**

Control user registration behavior:

```env
# Enable or disable user registrations
ALLOW_REGISTRATIONS=true

# Restrict signup to specific email domains (comma-separated)
FILTER_SIGNUP_DOMAINS=tyk.io
```

---

#### **System Settings**

Core system configuration:

```env
# Base URL where the portal is hosted
SITE_URL=http://localhost:3000

# License key for Tyk AI Portal
TYK_AI_LICENSE=XXXX

# Secret key for internal operations
TYK_AI_SECRET_KEY=your_secret_key
```

---

#### **Important Notes**

1. Always use strong, unique values for `TYK_AI_SECRET_KEY` in production environments
2. When using PostgreSQL in production, ensure your database connection string includes proper authentication credentials
3. SMTP configuration is required for user registration and notification features
4. The `SITE_URL` should reflect your production URL when deployed

For production deployments, it's recommended to:
- Use PostgreSQL as the database
- Configure proper SMTP settings for reliable email delivery
- Set appropriate domain restrictions using `FILTER_SIGNUP_DOMAINS` if needed
- Ensure all sensitive credentials are securely stored
