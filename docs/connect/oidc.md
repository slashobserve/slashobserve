---
title: Sign-in with OIDC
description: Set up sign-in through an OpenID Connect identity provider and decide who may sign in.
---

People sign in through an OIDC identity provider; there are no local passwords.
Create a client in your provider and give SlashObserve its settings:

- Register the redirect URI `<SLASHOBSERVE_PUBLIC_URL>/api/v1/auth/callback`.
- Use the authorization code flow.
  The server sends PKCE (S256) and a nonce, and asks for the `openid`, `email` and `profile` scopes.
- Set `SLASHOBSERVE_OIDC_ISSUER_URL` to the issuer exactly as the provider reports it in its discovery document.
  The server fetches `<issuer>/.well-known/openid-configuration` on the first sign-in attempt, not at startup, so a provider that is down when the server boots does not stop it.
- If the server reaches the provider at a different address than browsers do (for example by a Docker service name), set `SLASHOBSERVE_OIDC_INTERNAL_URL` to that origin.
  Only the server's own calls to the provider are rewritten; the issuer and the browser redirects keep the public address.
  The demo stack does this: browsers use `http://localhost:5556/dex` and the server uses `http://dex:5556`.

`SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` decides who gets in:

- A comma-separated list such as `example.com,example.org`.
  A person may sign in when the provider says their email is verified and its domain is in the list; the match is exact, so `example.com` does not admit `sub.example.com`.
- `*` allows every account the provider authenticates that has an email address, verified or not, and logs a warning at startup.
  Use it only when the provider itself is the access control, for example a private tenant.
- The setting is required, so a deployment can never be open by accident.

Users are identified by the provider's issuer and subject, and are created on first sign-in.
