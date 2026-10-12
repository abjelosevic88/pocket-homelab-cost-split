# Reverse proxy

The app serves plain HTTP on `:8080` and is happy behind any proxy. Set `SECURE_COOKIES=true` once HTTPS terminates in front of it. Request bodies are small JSON; no special body limits are needed.

## Caddy

```
split.example.com {
	reverse_proxy pocket-split:8080
}
```

With forward-auth (Tinyauth / Authelia / Authentik) and `AUTH_MODE=proxy` — users are created on first visit, the first one becomes admin:

```
split.example.com {
	forward_auth tinyauth:3000 {
		uri /api/auth/caddy
		copy_headers Remote-User Remote-Email
	}
	reverse_proxy pocket-split:8080
}
```

## nginx

```nginx
server {
    listen 443 ssl http2;
    server_name split.example.com;
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Traefik labels

```yaml
labels:
  - traefik.enable=true
  - traefik.http.routers.split.rule=Host(`split.example.com`)
  - traefik.http.routers.split.entrypoints=websecure
  - traefik.http.routers.split.tls.certresolver=le
  - traefik.http.services.split.loadbalancer.server.port=8080
```

`/metrics` is unauthenticated by default. Either set `METRICS_ENABLED=false` or block the path at the proxy if the host is reachable from the internet.
