# Running behind Cloudflare

Apps from this template can run behind Cloudflare in one of two ways: through
Cloudflare Tunnel (recommended), or with proxied DNS pointing at a public
origin. Either way, set things up in the app's config first, then on the
Cloudflare dashboard.

## Contents

- [Tunnel (recommended)](#tunnel-recommended)
- [Proxied DNS](#proxied-dns)
- [On the Cloudflare dashboard](#on-the-cloudflare-dashboard)
- [What the app already does for you](#what-the-app-already-does-for-you)

## Tunnel (recommended)

`cloudflared` runs next to the app and dials out to Cloudflare, so the origin
has no open port at all. Only `cloudflared` can reach the app, which is what
makes the visitor's address in `CF-Connecting-IP` safe to believe.

In `config.yaml`:

```yaml
server:
  trust_proxy: true
  trusted_proxies: [127.0.0.1, "::1"]  # cloudflared on the same host
  proxy_header: CF-Connecting-IP
  secure_cookies: true
  public_url: https://app.example.com
```

- With the app in Docker and `cloudflared` on the host, list the exact bridge
  gateway instead (`[172.17.0.1]` on the default bridge; find it with
  `docker network inspect bridge`). Do not list `172.16.0.0/12`: it trusts
  every container on every bridge network. A range wider than /8 (IPv4) or
  /16 (IPv6) is refused anyway.
- With `cloudflared` in its own container on a user-defined network, list
  that container's fixed address.
- Then check on the Logs screen that real visitor addresses show up, not
  `127.0.0.1` or the gateway. Until `trust_proxy` is right, every visitor
  shares the proxy's address and one rate-limit budget: a burst of sign-in
  attempts from anyone locks everyone out. The server says so once in its log
  (`[rate-limit] requests carry CF-Connecting-IP but server.trust_proxy is
  off ...`) when it sees the header with `trust_proxy` off.

## Proxied DNS

An orange-cloud DNS record pointing at a public IP. The origin is reachable
from the whole internet unless you close it, so:

- **TLS: Full (strict)**, with a Cloudflare Origin CA certificate on the
  origin. Never **Flexible**: Cloudflare would talk plain HTTP to the origin,
  and `secure_cookies` cookies would cross that leg in the clear.
- **Authenticated Origin Pulls** on, with the origin requiring Cloudflare's
  client certificate. Trusting Cloudflare's IP ranges means trusting every
  Cloudflare customer: anyone can put their own zone in front of your origin
  and send whatever `CF-Connecting-IP` they like. The client certificate is
  what proves the request came through *your* zone.
- **Firewall the origin** so only Cloudflare's ranges
  (<https://www.cloudflare.com/ips/>) reach the port.
- `trusted_proxies` then lists Cloudflare's ranges (or, better, the address of
  the local TLS terminator that checks the client certificate), with
  `proxy_header: CF-Connecting-IP`.

## On the Cloudflare dashboard

- **Caching: bypass the hostname.** Add a Cache Rule that bypasses cache for
  the app's hostname, and turn on **Cache Deception Armor**. The app sends
  `Cache-Control: no-store, private` on every API reply and error page, but a
  "cache everything" rule overrides that, and by default Cloudflare caches any
  path that ends in what looks like a file extension: a day of the owner's log,
  `/api/owner/logs/days/<date>`, could be served to the next visitor.
- **Turn off** Rocket Loader, Email Obfuscation and automatic analytics
  (Web Analytics / Zaraz) injection. They rewrite the page, and the app's
  Content-Security-Policy (`script-src 'self'`) blocks what they inject, so at
  best they do nothing and at worst they break the page.
- **Rate-limit rule** on `POST /api/login*` (and `/api/join/*` with invite
  links). The app limits per address too; Cloudflare's rule stops a flood
  before it reaches the origin.
- **Keep challenges off `/api/*`** when API keys or MCP are used: a script
  cannot solve a Managed Challenge or Bot Fight Mode page, and gets HTML where
  it expected JSON. Leave Bot Fight Mode off or skip it for `/api/*` and
  `/mcp`.
- **Leave Cloudflare's HSTS off.** The app sends its own
  `Strict-Transport-Security` when `secure_cookies` is on; two sources for one
  header only make it harder to change.
- **WebSockets / streaming**: the live log stream is Server-Sent Events, which
  works through Cloudflare; nothing to switch on.

## What the app already does for you

- `Cache-Control: no-store, private` and `Vary: Cookie, Authorization` on
  every response the static file server did not serve.
- The session cookie is `__Host-<cookie_name>` while `secure_cookies` is on, so
  an app on a sibling subdomain of the same zone cannot plant one.
- The rate limiter counts an IPv6 visitor by its /64, and logs the first 429 a
  budget gives in each window.
