# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.10.x+ (latest release line) | :white_check_mark: |
| < 0.10  | :x:                |

## Reporting a Vulnerability

We (no, there is not a mouse in my pocket) take security vulnerabilities seriously. If you discover a security issue, please report it responsibly.

### Security Updates
- Added prompt injection protections 3/19/26.  Additional info coming soon. 

### How to Report

**Please do NOT report security vulnerabilities through public GitHub issues.**

Instead, please report them via email to:

**<security@mdemg.dev>** (or create a private security advisory on GitHub)

### What to Include

Please include the following information in your report:

- Type of vulnerability (e.g., input validation issues, access control problems, data exposure)
- Full paths of source file(s) related to the vulnerability
- Location of the affected source code (tag/branch/commit or direct URL)
- Step-by-step instructions to reproduce the issue
- Proof-of-concept demonstrating the issue (if possible)
- Impact of the issue, including how an attacker might leverage it

### Response Timeline

- **Initial Response**: Within 48 hours of submission
- **Status Update**: Within 7 days with assessment
- **Resolution Target**: Critical vulnerabilities within 30 days

### What to Expect

1. **Acknowledgment**: We'll confirm receipt of your report
2. **Assessment**: We'll investigate and determine the severity
3. **Updates**: We'll keep you informed of our progress
4. **Resolution**: We'll work on a fix and coordinate disclosure
5. **Credit**: We'll credit you in the security advisory (unless you prefer anonymity)

## Security Best Practices for Users

### API Keys and Credentials

- Never commit `.env` files or API keys to version control
- Use environment variables for all sensitive configuration
- Rotate credentials regularly
- Use separate credentials for development and production

### Neo4j Database

- Change default passwords immediately after installation
- Enable authentication in production deployments
- Use TLS/SSL for database connections in production
- Restrict network access to the Neo4j port

### Embedding Providers

- Use separate API keys for development and production
- Monitor API usage for unexpected patterns
- Set appropriate rate limits and spending caps

### Deployment

- Run MDEMG behind a reverse proxy (nginx, traefik) in production
- Enable TLS for all API endpoints
- Implement proper authentication for API access
- Use network segmentation to isolate components

## Security Features

MDEMG includes several security features:

- **Protected Spaces**: The `mdemg-dev` space is protected from deletion
- **Input Validation**: API inputs are validated before processing
- **No Credential Storage**: MDEMG does not store user credentials
- **Audit Logging**: Operations are logged for audit purposes

## Vulnerability Disclosure Policy

We follow a coordinated disclosure process:

1. Reporter submits vulnerability privately
2. We acknowledge and investigate
3. We develop and test a fix
4. We release the fix and publish a security advisory
5. Reporter may publish details after the fix is released

We appreciate your help in keeping MDEMG and its users safe.
