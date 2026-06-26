# Runbook: wildcard cert for `*.preview.<your-domain>`

By default Deployik runs certbot per preview deploy to get a Let's Encrypt cert
for each auto-generated preview subdomain. On a busy instance you can instead
issue a single **wildcard** certificate for `*.preview.<your-domain>` and have
Deployik serve it to every single-label preview subdomain (skipping certbot per
deploy). Issuance and renewal of the wildcard are operated by you, not by
Deployik.

This assumes `BASE_DOMAIN=<your-domain>` (so preview domains are
`*.preview.<your-domain>`). Substitute your real base domain everywhere below;
the examples use `example.com`.

## Mount mapping (one host dir, two container views)
`/opt/nginx-proxy/certs` (host) is mounted into the **nginx** proxy container as
`/etc/nginx/certs` and into the **certbot** container as `/etc/letsencrypt`. A
cert certbot writes to `/etc/letsencrypt/live/<name>/` is read by nginx at
`/etc/nginx/certs/live/<name>/`. `PROXY_SSL_CERT`/`KEY` are the **nginx** paths.

## 1. Issue the wildcard (DNS-01)
A wildcard cert **requires** the DNS-01 challenge (HTTP-01 cannot validate a
wildcard), so you must be able to create a TXT record in your domain's DNS.
Either use the certbot DNS plugin for your provider (Cloudflare, Route53, etc.)
for automatic renewal, or `--manual` for a one-off:

    docker run --rm -it \
      -v /opt/nginx-proxy/certs:/etc/letsencrypt \
      certbot/certbot certonly --manual --preferred-challenges dns \
      --agree-tos -m admin@example.com \
      --cert-name wildcard.preview.example.com \
      -d '*.preview.example.com'

Create the `_acme-challenge.preview.example.com` TXT record at your DNS provider
when prompted. Result:
`/opt/nginx-proxy/certs/live/wildcard.preview.example.com/{fullchain,privkey}.pem`.

## 2. Configure Deployik (env)
    PROXY_SSL_CERT=/etc/nginx/certs/live/wildcard.preview.example.com/fullchain.pem
    PROXY_SSL_KEY=/etc/nginx/certs/live/wildcard.preview.example.com/privkey.pem
    PROXY_SSL_WILDCARD_DOMAINS=preview.example.com
Restart Deployik so the domain Manager picks them up.

## 3. Verify
Deploy any preview project (e.g. `my-api`). The deploy log should show
"Using wildcard certificate for …" (no certbot run), and the generated vhost
(`/opt/nginx-proxy/conf.d/deployik-<domain>.conf`) should reference the wildcard
cert path. `curl -I https://<sub>.preview.example.com/` should return a valid TLS
response.

## 4. Renewal
A `--manual` DNS-01 cert does not auto-renew. Either re-run step 1 before expiry
(~60 days), or use a provider DNS plugin with a renewal hook, then
`docker exec nginx-proxy nginx -s reload`. A failed renewal degrades gracefully —
the existing cert serves until expiry.

## Rollback
Unset `PROXY_SSL_WILDCARD_DOMAINS` (or `PROXY_SSL_CERT`) and restart Deployik;
the next deploy/reconcile reverts to per-domain certbot.
