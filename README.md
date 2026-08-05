# ShoreOS FIRE

ShoreOS FIRE is the first runnable ShoreOS personal service for FIRE planning.

The current frontend started as a single-file prototype and is now served by a Go API service with embedded static files.

## Run Locally

Apply the MySQL schemas first:

```bash
mysql -uroot -p < schema/mysql/001_shoreos_fire.sql
mysql -uroot -p < schema/mysql/002_ledger_v2.sql
mysql -uroot -p < schema/mysql/003_identity_and_human_ledger.sql
mysql -uroot -p < schema/mysql/004_ledger_budget_and_fire_projection.sql
```

Create `.env`:

```bash
cp .env.example .env
```

Prepare the isolated ledger parser runtime once:

```bash
python3 -m venv .venv-ledger
.venv-ledger/bin/python -m pip install -r tools/requirements-ledger.txt
```

Run:

```bash
go run ./cmd/server
```

Open:

```text
http://127.0.0.1:8090/
```

H5 用户可通过 `POST /api/v1/auth/register` 显式注册用户名和密码。注册默认开启；
私有单用户部署可设置 `SHOREOS_ENABLE_REGISTRATION=false` 关闭。密码只保存 bcrypt 哈希。

## Deploy

See:

- `docs/deploy_guide.md`
- `docs/cnb_mirror.md`
- `docs/identity-and-human-ledger.md`
- `docs/shared-ledger-fire-contract-v2.md`
