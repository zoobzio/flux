# Security Policy

## Supported Versions

We release patches for security vulnerabilities. Which versions are eligible for receiving such patches depends on the CVSS v3.0 Rating:

| Version | Supported          | Status |
| ------- | ------------------ | ------ |
| latest  | Yes | Active development |
| < latest | No | Security fixes only for critical issues |

## Reporting a Vulnerability

We take the security of flux seriously. If you have discovered a security vulnerability in this project, please report it responsibly.

### How to Report

**Please DO NOT report security vulnerabilities through public GitHub issues.**

Instead, please report them via one of the following methods:

1. **GitHub Security Advisories** (Preferred)
   - Go to the [Security tab](https://github.com/zoobzio/flux/security) of this repository
   - Click "Report a vulnerability"
   - Fill out the form with details about the vulnerability

2. **Email**
   - Send details to the repository maintainer through GitHub profile contact information
   - Use PGP encryption if possible for sensitive details

### What to Include

Please include the following information (as much as you can provide) to help us better understand the nature and scope of the possible issue:

- **Type of issue** (e.g., race condition, deadlock, memory leak, etc.)
- **Full paths of source file(s)** related to the manifestation of the issue
- **The location of the affected source code** (tag/branch/commit or direct URL)
- **Any special configuration required** to reproduce the issue
- **Step-by-step instructions** to reproduce the issue
- **Proof-of-concept or exploit code** (if possible)
- **Impact of the issue**, including how an attacker might exploit the issue
- **Your name and affiliation** (optional)

### What to Expect

- **Acknowledgment**: We will acknowledge receipt of your vulnerability report within 48 hours
- **Initial Assessment**: Within 7 days, we will provide an initial assessment of the report
- **Resolution Timeline**: We aim to resolve critical issues within 30 days
- **Disclosure**: We will coordinate with you on the disclosure timeline

### Preferred Languages

We prefer all communications to be in English.

## Security Best Practices

When using flux in your applications, we recommend:

1. **Keep Dependencies Updated**
   ```bash
   go get -u github.com/zoobzio/flux
   ```

2. **Validate Configuration**
   - Always implement thorough validation functions
   - Check for security-sensitive fields (ports, URLs, credentials)
   - Reject configuration that could compromise security

3. **Error Handling**
   - Implement proper error handling in transform/validate/apply functions
   - Log validation failures for audit purposes
   - Don't expose sensitive config details in error messages

4. **File Permissions**
   - Ensure config files have appropriate permissions
   - Don't store secrets in plain config files
   - Use environment variables or secret managers for sensitive data

5. **State Management**
   - Monitor Degraded state for potential issues
   - Alert on repeated validation failures
   - Review configuration changes in audit logs

## Security Features

flux includes several built-in security features:

- **Validation Before Apply**: Invalid config never reaches application
- **State Isolation**: Failed updates don't corrupt current config
- **Rollback Protection**: Previous valid config retained on failure
- **Capitan Signals**: Full audit trail of configuration changes
- **Debouncing**: Prevents rapid config thrashing

## Automated Security Scanning

This project uses:

- **CodeQL**: GitHub's semantic code analysis for security vulnerabilities
- **golangci-lint**: Static analysis including security linters
- **Codecov**: Coverage tracking to ensure security-critical code is tested

## Vulnerability Disclosure Policy

- Security vulnerabilities will be disclosed via GitHub Security Advisories
- We follow a 90-day disclosure timeline for non-critical issues
- Critical vulnerabilities may be disclosed sooner after patches are available
- We will credit reporters who follow responsible disclosure practices

## Credits

We thank the following individuals for responsibly disclosing security issues:

_This list is currently empty. Be the first to help improve our security!_

---

**Last Updated**: 2024-12-03
