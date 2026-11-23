---
title: Sign-in with OIDC
description: Set up sign-in through an OpenID Connect identity provider and decide who may sign in.
---

People sign in through an OIDC identity provider; there are no local passwords.
A server with no identity provider has no browser sign-in and takes only service-account keys; see [Run with no browser sign-in](../operate/service-account-keys.md#run-with-no-browser-sign-in).

## Configure the client

Create a client in your provider and give SlashObserve its settings:

- Register the redirect URI `<SLASHOBSERVE_PUBLIC_URL>/api/v1/auth/callback`.
- Use the authorization code flow.
  The server sends PKCE (S256) and a nonce, and asks for the `openid`, `email` and `profile` scopes.
- Set `SLASHOBSERVE_OIDC_ISSUER_URL` to the issuer exactly as the provider reports it in its discovery document.
  The server fetches `<issuer>/.well-known/openid-configuration` on the first sign-in attempt, not at startup, so a provider that is down when the server boots does not stop it.
  While discovery fails, the sign-in page shows an error and the log has `oidc discovery failed` with `error_code` `oidc_unavailable`.
- If the server reaches the provider at a different address than browsers do (for example by a Docker service name), set `SLASHOBSERVE_OIDC_INTERNAL_URL` to that origin.
  Only the server's own calls to the provider are rewritten; the issuer and the browser redirects keep the public address.
  The demo stack does this: browsers use `http://localhost:55556/dex` and the server uses `http://dex:5556`.

## Decide who may sign in

`SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` decides who gets in:

- A comma-separated list such as `example.com,example.org`.
  A person may sign in when the provider says their email is verified and its domain is in the list; the match is exact, so `example.com` does not admit `sub.example.com`.
- `*` allows every account the provider authenticates that has an email address, verified or not, and logs a warning at startup.
  Use it only when the provider itself is the access control, for example a private tenant.
  An address the provider does not report as verified never makes a person an Admin through `SLASHOBSERVE_ADMIN_EMAILS`, even under `*`, and such a sign-in logs the warning `admin email signed in unverified`; with a provider that does not send `email_verified`, give Admin through a group instead; see [Roles](../teams/roles.md#how-a-role-is-decided).
- The setting is required, so a deployment can never be open by accident.

A refused sign-in sends the person back to the sign-in page with "This account is not allowed to sign in." and logs `sign-in refused` with `reason` `domain_not_allowed` or `email_unverified` and the email's domain, never the address.
A provider that sends no email address at all is refused too, whatever the setting.

An [invite](../teams/people.md#invite-people) does not change these rules: an email outside the list cannot be invited, and an invited role applies only to a first sign-in with a verified email.
An Admin can also [deactivate](../teams/people.md#deactivate-a-person) a person, and their next sign-in is refused with "This account was deactivated by an Admin."

## Users and groups

Users are identified by the provider's issuer and subject, and are created on first sign-in.
A session lasts `SLASHOBSERVE_SESSION_TTL` from sign-in, 7 days by default and between 5 minutes and 30 days.
Set `SLASHOBSERVE_AUTH_PROVIDER_NAME` to the name people know the provider by, up to 64 characters; the web app shows it in the account menu, on the People page and in the access dialogs.
Settings shows the issuer, the allowed domains, the provider name and the session length, read-only.
The server also asks for the `groups` scope when the provider lists it in `scopes_supported`, so that `SLASHOBSERVE_OIDC_GROUP_ROLES` can map groups to roles (see [Roles](../teams/roles.md)).
