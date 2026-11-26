#!/bin/bash
set -euo pipefail

# ============================================================
# PostgreSQL Backup Script
# Intended for cron: 0 3 * * * /path/to/backup_db.sh
# ============================================================

BACKUP_DIR="${BACKUP_DIR:-./backups}"
DB_NAME="${POSTGRES_DB:-saas_ecommerce}"
DB_USER="${POSTGRES_USER:-postgres}"
RETAIN_DAYS="${RETAIN_DAYS:-14}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
FILENAME="${BACKUP_DIR}/${DB_NAME}_${TIMESTAMP}.sql.gz"

mkdir -p "$BACKUP_DIR"

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Starting backup: $FILENAME"

# Use docker exec to dump from the running container
docker compose exec -T db pg_dump \
    -U "$DB_USER" \
    -d "$DB_NAME" \
    --clean \
    --if-exists \
    --no-owner \
    --no-privileges \
    | gzip > "$FILENAME"

# Verify backup is non-empty
if [[ ! -s "$FILENAME" ]]; then
    echo "[ERROR] Backup file is empty, removing."
    rm -f "$FILENAME"
    exit 1
fi

# Clean up old backups
delete_count=$(find "$BACKUP_DIR" -maxdepth 1 -name "${DB_NAME}_*.sql.gz" -mtime +$RETAIN_DAYS -print -delete | wc -l)

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Backup complete: $FILENAME"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Removed $delete_count backup(s) older than $RETAIN_DAYS days."
