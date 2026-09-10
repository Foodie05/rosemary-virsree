#!/usr/bin/env bash
set -euo pipefail

DEPLOY_HOST="${RVS_DEPLOY_HOST:-cruty.cn}"
PUBLIC_URL="${RVS_PUBLIC_URL:-https://storage.cruty.cn}"
REMOTE_ENV="/etc/rosemary-virsree.env"

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing required command: $1" >&2; exit 1; }; }
need ssh
need scp

read -r -p "OIDC client ID: " OIDC_CLIENT_ID
read -r -s -p "OIDC client secret: " OIDC_CLIENT_SECRET
echo
read -r -p "Whitelisted administrator email: " ADMIN_EMAIL

for value in "$OIDC_CLIENT_ID" "$OIDC_CLIENT_SECRET" "$ADMIN_EMAIL"; do
  [[ -n "$value" && "$value" != *$'\n'* && "$value" != *$'\r'* ]] || { echo "Values must be non-empty single lines." >&2; exit 1; }
done
[[ "$ADMIN_EMAIL" == *@*.* ]] || { echo "The administrator email does not look valid." >&2; exit 1; }

quote_env() {
  local value=${1//\\/\\\\}
  value=${value//\"/\\\"}
  printf '"%s"' "$value"
}

TMP_ENV=$(mktemp "${TMPDIR:-/tmp}/rosemary-env.XXXXXX")
REMOTE_TMP="/tmp/rosemary-virsree.env.$RANDOM"
cleanup() { chmod 600 "$TMP_ENV" 2>/dev/null || true; rm -f "$TMP_ENV"; ssh "$DEPLOY_HOST" "rm -f '$REMOTE_TMP'" >/dev/null 2>&1 || true; }
trap cleanup EXIT

{
  echo 'RVS_LISTEN=127.0.0.1:18741'
  echo "RVS_PUBLIC_URL=$(quote_env "$PUBLIC_URL")"
  echo 'RVS_DATABASE=/var/lib/rosemary-virsree/rosemary.db'
  echo 'RVS_WEB_DIR=/opt/rosemary-virsree/current/web/dist'
  echo 'RVS_DOCS_DIR=/opt/rosemary-virsree/current/docs'
  echo 'RVS_DOWNLOAD_DIR=/opt/rosemary-virsree/current/downloads'
  echo 'RVS_PROJECT_URL=https://github.com/Foodie05/rosemary-virsree'
  echo 'RVS_RELEASE_URL=https://github.com/Foodie05/rosemary-virsree/releases'
  echo 'RVS_OIDC_ISSUER=https://apiauth.cruty.cn'
  echo "RVS_OIDC_CLIENT_ID=$(quote_env "$OIDC_CLIENT_ID")"
  echo "RVS_OIDC_CLIENT_SECRET=$(quote_env "$OIDC_CLIENT_SECRET")"
  echo "RVS_OIDC_REDIRECT_URL=$(quote_env "$PUBLIC_URL/auth/callback")"
  echo "RVS_ADMIN_EMAILS=$(quote_env "$ADMIN_EMAIL")"
  echo 'RVS_SESSION_TTL_SECONDS=43200'
  echo 'RVS_TOTAL_QUOTA=1099511627776'
  echo 'RVS_MAX_OBJECTS_PER_BUCKET=1000000'
  echo 'RVS_MAX_PENDING_UPLOADS_PER_BUCKET=1000'
  echo 'RVS_BACKUP_INTERVAL_SECONDS=21600'
  echo 'RVS_BACKUP_RETENTION=7'
} > "$TMP_ENV"
chmod 600 "$TMP_ENV"

echo "Uploading the protected configuration over SSH to $DEPLOY_HOST..."
scp -q "$TMP_ENV" "$DEPLOY_HOST:$REMOTE_TMP"
ssh "$DEPLOY_HOST" bash -s -- "$REMOTE_TMP" "$REMOTE_ENV" <<'REMOTE'
set -euo pipefail
incoming=$1
destination=$2
merged="${incoming}.merged"
admin_line=""
master_line=""
if [[ -f "$destination" ]]; then
  admin_line=$(grep -m1 '^RVS_ADMIN_TOKEN=' "$destination" || true)
  master_line=$(grep -m1 '^RVS_MASTER_KEY=' "$destination" || true)
fi
if [[ -z "$admin_line" ]]; then admin_line="RVS_ADMIN_TOKEN=$(openssl rand -base64 36 | tr -d '\n')"; fi
if [[ -z "$master_line" ]]; then master_line="RVS_MASTER_KEY=$(openssl rand -base64 48 | tr -d '\n')"; fi
grep -Ev '^RVS_(ADMIN_TOKEN|MASTER_KEY)=' "$incoming" > "$merged"
printf '%s\n%s\n' "$admin_line" "$master_line" >> "$merged"
install -o root -g rosemary -m 0640 "$merged" "$destination"
rm -f "$incoming" "$merged"
systemctl restart rosemary-virsree
systemctl --no-pager --full status rosemary-virsree | sed -n '1,12p'
REMOTE
echo
echo "Configuration installed. Open: $PUBLIC_URL"
echo "The management and encryption keys were preserved (or generated once) without being printed. Back up $REMOTE_ENV securely on the server."
