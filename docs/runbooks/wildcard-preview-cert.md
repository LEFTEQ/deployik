# Runbook: wildcard cert for `*.preview.example.com`

Deployik serves this one cert to every single-label preview subdomain (skipping
certbot per deploy). Issuance/renewal is operated here, not by Deployik.

## Mount mapping (one host dir, two container views)
`/opt/nginx-proxy/certs` (host) is mounted into the **nginx** proxy container as
`/etc/nginx/certs` and into the **certbot** container as `/etc/letsencrypt`. A
cert certbot writes to `/etc/letsencrypt/live/<name>/` is read by nginx at
`/etc/nginx/certs/live/<name>/`. `PROXY_SSL_CERT`/`KEY` are the **nginx** paths.

## 1. Issue the wildcard (DNS-01, GoDaddy)
`example.com` DNS is GoDaddy (`pdns0{7,8}.domaincontrol.com`). DNS-01 is required
(HTTP-01 cannot validate a wildcard). Either use a GoDaddy certbot DNS plugin, or
`--manual`:

    docker run --rm -it \
      -v /opt/nginx-proxy/certs:/etc/letsencrypt \
      certbot/certbot certonly --manual --preferred-challenges dns \
      --agree-tos -m admin@example.com \
      --cert-name wildcard.preview.example.com \
      -d '*.preview.example.com'

Place the `_acme-challenge.preview.example.com` TXT record in GoDaddy when prompted.
Result: `/opt/nginx-proxy/certs/live/wildcard.preview.example.com/{fullchain,privkey}.pem`.

## 2. Configure Deployik (env)
    PROXY_SSL_CERT=/etc/nginx/certs/live/wildcard.preview.example.com/fullchain.pem
    PROXY_SSL_KEY=/etc/nginx/certs/live/wildcard.preview.example.com/privkey.pem
    PROXY_SSL_WILDCARD_DOMAINS=preview.example.com
Restart Deployik so the domain Manager picks them up.

## 3. Verify
Deploy any preview project (e.g. `acme-app-api`). The deploy log should show
"Using wildcard certificate for …" (no certbot run), and the generated vhost
(`/opt/nginx-proxy/conf.d/deployik-<domain>.conf`) should reference the wildcard
cert path. `curl -I https://<sub>.preview.example.com/` should return a valid TLS
response.

## 4. Renewal
`--manual` DNS-01 does not auto-renew. Either re-run step 1 before expiry (~60
days) or script a GoDaddy DNS-01 renewal hook, then `docker exec nginx-proxy
nginx -s reload`. A failed renewal degrades gracefully — the existing cert serves
until expiry.

## Rollback
Unset `PROXY_SSL_WILDCARD_DOMAINS` (or `PROXY_SSL_CERT`) and restart Deployik;
the next deploy/reconcile reverts to per-domain certbot.
