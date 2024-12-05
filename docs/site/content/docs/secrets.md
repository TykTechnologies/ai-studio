---
title: "Secrets"
weight: 50
# bookFlatSection: false
# bookToc: true
# bookHidden: false
# bookCollapseSection: false
# bookComments: false
# bookSearchExclude: false
---

### Secrets View

The **Secrets View** allows administrators to securely store sensitive information, such as API keys or passwords, which can be referenced throughout the Tyk AI Portal. These secrets are encrypted at rest using the environment variable key `TYK_AI_SECRET_KEY`. Below is a breakdown of the functionality and use cases for this view.

---

#### **Adding a Secret**

1. **Variable Name** *(Required)*:
   - The name used to reference the secret in the system (e.g., `MYSECRET`).
   - Only letters, numbers, and underscores are allowed.

2. **Secret Value** *(Required)*:
   - The sensitive information or password to be securely stored.
   - The value is masked but can be viewed using the **eye icon** toggle.

3. **Generated Reference**:
   - Each secret is assigned a unique reference path that can be used throughout the Admin interface.
   - Example: `$SECRET/MYSECRET`

4. **Add Secret Button**:
   - Saves the secret to the system with encryption. The secret becomes immediately available for use.

---

#### **Use Cases for Secrets**

1. **Password Fields**:
   - Secrets can be referenced in password fields across the Admin interface, ensuring sensitive information is not hardcoded or exposed.

2. **Environment Isolation**:
   - Secrets are tied to the specific `TYK_AI_SECRET_KEY` environment variable, providing isolation and security for different environments (e.g., development, staging, production).

3. **Centralized Management**:
   - All sensitive data is stored and managed in one location, making it easier to update or rotate credentials securely.

---

#### **Encryption and Security**

1. **Encrypted at Rest**:
   - Secrets are securely stored using encryption, ensuring they are protected even if the database is compromised.

2. **Environment Variable Key**:
   - The encryption process relies on the `TYK_AI_SECRET_KEY` environment variable, making it essential to configure and protect this key in your deployment.

3. **Restricted Access**:
   - Secrets can only be accessed by authorized users with administrative privileges.

---

#### **Purpose and Benefits**

1. **Improved Security**:
   - By storing secrets securely and encrypting them at rest, the portal minimizes the risk of accidental exposure or misuse of sensitive information.

2. **Ease of Use**:
   - Administrators can reference secrets in configurations without having to expose sensitive details in plaintext.

3. **Compliance and Governance**:
   - Helps meet organizational and regulatory requirements for secure data handling.

---

The **Secrets View** is a critical component of the Tyk AI Portal, providing a secure, centralized solution for managing sensitive information while ensuring compliance and maintaining best practices in security.
