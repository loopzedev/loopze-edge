# Security Policy

## Reporting a vulnerability

If you discover a security vulnerability in LOOPZE, please report it **privately** so it can be fixed before public disclosure.

**Email:** dennis.bleul@googlemail.com

Please include:

- A description of the vulnerability and the affected component
- Steps to reproduce, or a proof of concept if available
- Your assessment of impact (data exposure, privilege escalation, denial of service, etc.)
- Any suggested mitigation

## What to expect

- **Acknowledgement** within 5 business days
- An initial assessment and severity rating within 14 days
- A coordinated disclosure timeline once a fix is available — typically released alongside the patched version
- Credit to you in the release notes, if you wish

## Out of scope

- Issues that require physical access to the device running LOOPZE
- Vulnerabilities in third-party dependencies — please report these upstream; we monitor `go.sum` and frontend dependencies and update reactively
- Brute-force attacks against `/api/v1/auth/login` (rate limiting is in place; see `internal/auth/throttle.go`)

## Supported versions

Until v1.0.0, only the latest commit on `main` receives security patches. Tagged releases will be pinned and patched once the project reaches v1.0.

## Public issues

Please do **not** open a public GitHub issue for security vulnerabilities. Use the email address above.
