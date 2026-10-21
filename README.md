# printodo-api

Backend for the Printodo system: a phone app posts short notes, a thermal
receipt-printer worker prints them, and a web admin site lets you browse what
you printed (by date), classify each note into categories, and jump to the
official site of any brand/product mentioned.

## Components that talk to this API

- **PrintTodo** (mobile app): `POST /login` then `POST /message`.
- **printodo-worker** (printer): `POST /login`, `GET /messages`, `DELETE /messages`.
- **Admin website** (browser): `GET /admin` and friends, cookie-authenticated.

## JSON API (unchanged contracts)

1. `POST /login` with `{"username": "...", "password": "..."}` → `{"token": "..."}`.
2. `POST /message` with `{"message": "..."}` and header `Authorization: bearer <token>`.
   Records the note as history (which also queues it for the printer) and kicks
   off an asynchronous AI category/link suggestion.
3. `GET /messages` (worker, `User-Agent: todo-printer/1.0`) → `["note", ...]` of
   everything not yet printed.
4. `DELETE /messages` (worker) → marks the pending notes as printed. History is
   preserved (notes are never destroyed), so the admin site can show it forever.

## Admin website

- `GET /admin/login`, `POST /admin/login`, `GET /admin/logout` — cookie session
  (HttpOnly JWT), reuses the same username/password as the API.
- `GET /admin` — the view page (dark "Night Console" UI): print history grouped
  by date, category sidebar with filter + counts (scales to many categories),
  per-note brand/product links, and a "needs classifying" worklist with
  AI-suggested categories.
- `GET /admin/data` — JSON feed the view page consumes.
- `POST /admin/items/{id}/classify` — saves the chosen categories for a note.

## Storage

PostgreSQL via GORM. Tables (`items`, `categories`, `item_categories`) are
created automatically on startup (`AutoMigrate`). A note (`Item`) doubles as the
printer queue (`printed_at IS NULL`) and the permanent history.

### One-time migration from the old BoltDB

If a legacy `notes.db` (BoltDB) file is present at startup, every note in it is
imported into Postgres as an unprinted item and the file is renamed to
`notes.db.imported` so it won't re-import. Bolt stored no timestamps, so imported
items are stamped with the import time.

## Configuration (environment variables)

Database (matches the sibling `upkeep` app so it can share a Postgres server):

- `PG_HOST`, `PG_PORT` (5432), `PG_USER` (postgres), `PG_PASSWORD`, `PG_DB` (printodo)
- or `DATABASE_URL` (e.g. `postgres://printodo:printodo@localhost:5432/printodo?sslmode=disable`)

App:

- `USERNAME`, `PASSWORD` — the single login (defaults `test`/`test`).
- `JWT_SIGNING_KEY` — HMAC key for API and admin-cookie JWTs.
- `ENVIRONMENT` — `dev` (default) serves the admin cookie without the Secure flag
  so it works over plain HTTP; anything else sets Secure (use behind HTTPS).
- `PORT` — listen port (default 8000).
- `ANTHROPIC_API_KEY` — enables AI category suggestions and brand/product links
  (model `claude-haiku-4-5`). If unset, the app works fully; notes just arrive
  without suggestions and you classify them manually.

## Build & run

```sh
go build           # produces ./printodo-api
PG_HOST=... PG_PASSWORD=... ANTHROPIC_API_KEY=... ./printodo-api
```

`mockups/` contains standalone HTML design explorations for the view page and is
not part of the built binary.

## Deploy (Kubernetes + CI)

Manifests live in `k8s/` and follow the same pattern as the sibling `upkeep`
app:

- `deployment.yaml` — pulls the private image `ghcr.io/moosilauke18/printodo-api`
  (via `imagePullSecrets: ghcr-secret`) and wires Postgres to the shared
  `evanfarrell-db` cluster as its own `printodo` user, database `printodo`
  (`PG_HOST`/`PG_PORT` from the configMap `evanfarrell-db-private-connection`,
  `PG_USER`/`PG_PASSWORD` from the secret `printodo-db`, which
  `kubernetes/db-users/provision.sh` creates).
- `service.yaml`, `ingress.yaml` — `todo.evanfarrell.com`, TLS via cert-manager.
- `examples/secret.yaml` — template. Copy to `k8s/secret.yaml` (gitignored),
  fill in `password`, `jwt-signing-key`, `anthropic-api-key`, and apply once:
  `kubectl apply -f k8s/secret.yaml`.

`.github/workflows/deploy.yml` runs on push to `main`: test → build/push the
image to ghcr → `kubectl apply -f k8s/` and roll the deployment. It needs these
repo secrets: `DIGITALOCEAN_ACCESS_TOKEN`, `DO_CLUSTER_NAME` (`GITHUB_TOKEN` is
automatic). Cluster prerequisites that already exist for the other apps:
`ghcr-secret` (ghcr pull secret) and the `evanfarrell-db-*` operator objects.
The `printodo-secret` is applied manually (above); CI never touches it.

## TODO

- JWT refresh: https://medium.com/monstar-lab-bangladesh-engineering/jwt-auth-in-go-part-2-refresh-tokens-d334777ca8a0
