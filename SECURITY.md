# Reporting security issues

Read [the security model](spec/security.md) before
using it on a shared network. No hardened or production release is claimed.

Report suspected vulnerabilities privately through the repository host's private
vulnerability reporting feature when enabled. If it is unavailable, ask a
maintainer for a private reporting channel before sharing exploit details.
Do not post API keys, private prompts, packet captures with credentials, or
personal endpoint addresses in public issues.

Include the version, platform, relevant configuration with secrets removed,
reproduction steps, and the expected security boundary. Unsupported authentication
must fail without anonymous fallback. Any credential leakage, redirect following,
or cross-endpoint credential reuse is a security defect.
