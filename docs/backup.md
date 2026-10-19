# Backup & restore

Everything lives in `DATA_DIR` (`/data` in the container):

```
/data/pocket-split.db     # the SQLite database (WAL mode: -wal / -shm files appear while running)
/data/backups/            # VACUUM INTO copies made by the CLI
```

## Making a backup

- **CLI:** `docker exec pocket-split pocket-split backup` (or `backup -o /data/backups/named.db`).
- **Portable:** each group's Settings → Export → JSON: people, expenses, shares and stored exchange rates, re-importable on another instance (Settings → Backup & data → Import) as a new group.

All of these produce a consistent snapshot even while the app is writing. Copying `pocket-split.db` raw while the container runs can give you a torn file because of the WAL.

### With restic / Borg / Backrest / Kopia

Add `DATA_DIR` to your backup set, **exclude** `pocket-split.db*`, and run the CLI backup (or the sqlite3 `.backup` command) as a pre-snapshot hook so the consistent copy under `backups/` is what gets picked up.

## Restoring

```bash
docker compose stop pocket-split
cp pocket-split-20261012-030000.db data/pocket-split.db
rm -f data/pocket-split.db-wal data/pocket-split.db-shm
docker compose start pocket-split
```

Or start a fresh instance and import each group's JSON export.
