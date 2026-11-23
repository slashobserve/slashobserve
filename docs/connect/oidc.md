---
title: Sign-in with OIDC
description: Connect an OpenID Connect identity provider so people can sign in to the web app, and decide which email addresses may sign in.
---

People sign in to SlashObserve through an OpenID Connect (OIDC) identity provider such as Dex, Keycloak, Okta or Microsoft Entra ID.
There are no local passwords.
A server with no identity provider has no browser sign-in and takes only service-account keys; see [Run with no browser sign-in](../operate/service-account-keys.md#run-with-no-browser-sign-in).

## Connect an identity provider

1. In your provider, create a client for SlashObserve that uses the authorization code flow.
   Register the redirect URI `<SLASHOBSERVE_PUBLIC_URL>/api/v1/auth/callback`.
2. Set these variables on the server.
   Set all of the first three or none of them.

   ```text
   SLASHOBSERVE_OIDC_ISSUER_URL=https://idp.example.com
   SLASHOBSERVE_OIDC_CLIENT_ID=<client id>
   SLASHOBSERVE_OIDC_CLIENT_SECRET=<client secret>
   SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS=example.com
   ```

   `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` is required whenever sign-in is on, so a deployment is never open by accident.
   [Decide who may sign in](#decide-who-may-sign-in) explains its values.
3. Start the server.
   The log has `oidc login configured` with the issuer.
4. Open the web app and choose **Sign in**.
   Your provider asks you to authenticate, and you land back in SlashObserve signed in.

The first person to sign in becomes an Admin unless a role setting decides otherwise; see [Roles](../teams/roles.md#how-a-role-is-decided).

## Decide who may sign in

Set `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` to one of two things.

- A comma-separated list of domains, such as `example.com,example.org`.
  A person may sign in when the provider says their email is verified and the email's domain is in the list.
  The match is exact, so `example.com` does not admit `sub.example.com`.
- `*`, which allows every account the provider authenticates that has an email address, verified or not.
  The server logs the warning `all identity-provider accounts may sign in` at startup.
  Use it only when the provider is itself the access control, for example a private tenant.
  `*` cannot be combined with domains.

A refused person returns to the sign-in page with "This account is not allowed to sign in."
The log has `sign-in refused` with the `reason`, never the address.
For `domain_not_allowed` and `email_unverified` it also logs the email's domain.
The reasons are `domain_not_allowed`, `email_unverified`, `email_too_long` and `email_invalid`.
A provider that sends no email address at all is refused whatever the setting.
That case shows "Sign-in failed. Try again." and logs `sign-in failed` with `error_code` `email_missing`.

An [invite](../teams/people.md#invite-people) does not change these rules.
An email outside the list cannot be invited, and an invited role applies only to a first sign-in with a verified email.
An Admin can also [deactivate](../teams/people.md#deactivate-a-person) a person.
Their next sign-in is refused with "This account was deactivated by an Admin. Ask an Admin to reactivate it."

## Sign in when the server and browsers reach the provider differently

If the server reaches the provider at another address than browsers do, for example by a Docker service name, set `SLASHOBSERVE_OIDC_INTERNAL_URL` to that origin.
Only the server's own calls to the provider use it.
The issuer and the browser redirects keep the public address.
The demo stack does this: browsers use `http://localhost:5556/dex` and the server uses `http://dex:5556`.

## Check why sign-in fails

The server fetches `<issuer>/.well-known/openid-configuration` on the first sign-in attempt, not at startup.
A provider that is down when the server boots does not stop it.
While discovery fails, the sign-in page shows "The identity provider is unavailable. Try again shortly." and the log has `oidc discovery failed` with `error_code` `oidc_unavailable`.
Calls to the provider time out after 10 seconds.

Set `SLASHOBSERVE_OIDC_ISSUER_URL` to the issuer exactly as the provider reports it in its discovery document, because the server compares it exactly.

## Reference

### Settings

| Variable | Default | Notes |
| --- | --- | --- |
| `SLASHOBSERVE_OIDC_ISSUER_URL` | none | Absolute `http` or `https` URL. |
| `SLASHOBSERVE_OIDC_CLIENT_ID` | none | Set with the issuer and the secret, or not at all. |
| `SLASHOBSERVE_OIDC_CLIENT_SECRET` | none | Set with the issuer and the client ID, or not at all. |
| `SLASHOBSERVE_OIDC_INTERNAL_URL` | none | Origin the server uses for its own calls. |
| `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS` | none | Required with sign-in. Domains or `*`. |
| `SLASHOBSERVE_AUTH_PROVIDER_NAME` | none | Up to 64 characters. |
| `SLASHOBSERVE_SESSION_TTL` | `168h` | From 5 minutes to 720 hours. |
| `SLASHOBSERVE_OIDC_GROUP_ROLES` | none | Maps groups to roles; see [Roles](../teams/roles.md). |

With none of the issuer, client ID and client secret set, the server refuses to start if `SLASHOBSERVE_OIDC_INTERNAL_URL`, `SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS`, `SLASHOBSERVE_ADMIN_EMAILS`, `SLASHOBSERVE_OIDC_GROUP_ROLES` or `SLASHOBSERVE_AUTH_PROVIDER_NAME` is set, because only browser sign-in reads them.
Setting only some of the three is also refused.

### What the server asks the provider for

The server uses the authorization code flow with PKCE (S256) and a nonce.
It asks for the `openid`, `email` and `profile` scopes.
It also asks for `groups` when the provider lists it in `scopes_supported`, so that `SLASHOBSERVE_OIDC_GROUP_ROLES` can map groups to roles.

### Details of the email rules

Under a domain list, the provider must report the email as verified.
Under `*`, an unverified email may sign in, but it never makes a person an Admin through `SLASHOBSERVE_ADMIN_EMAILS`.
Such a sign-in logs the warning `admin email signed in unverified` with the person's user id.
With a provider that does not send `email_verified`, give Admin through a group instead; see [Roles](../teams/roles.md#how-a-role-is-decided).

### Users and sessions

Users are identified by the provider's issuer and subject, and are created on first sign-in.
A session lasts `SLASHOBSERVE_SESSION_TTL` from sign-in.
Set `SLASHOBSERVE_AUTH_PROVIDER_NAME` to the name people know the provider by.
The web app shows it in the account menu, on the People page and in the access dialogs.
**Settings** has a **Sign-in** card that shows the provider name, the identity provider, the allowed email domains and the session length, read-only.
