# ShoreOS FIRE

ShoreOS FIRE is the first runnable ShoreOS personal service for FIRE planning.

The current frontend started as a single-file prototype and is now served by a Go API service with embedded static files.

## Run Locally

Apply the MySQL schema first:

```bash
mysql -uroot -p < schema/mysql/001_shoreos_fire.sql
```

Create `.env`:

```bash
cp .env.example .env
```

Run:

```bash
go run ./cmd/server
```

Open:

```text
http://127.0.0.1:8090/
```

## Deploy

See `docs/deploy_guide.md`.
