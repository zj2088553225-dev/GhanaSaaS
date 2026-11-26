#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${ENV_FILE:-$PROJECT_DIR/.env.production}"
STATE_DIR="${STATE_DIR:-$PROJECT_DIR/.deploy}"
BACKUP_DIR="${BACKUP_DIR:-$PROJECT_DIR/backups/production}"
INVARIANT_SQL="${INVARIANT_SQL:-$PROJECT_DIR/docs/product-invariant-preflight.sql}"
HEALTH_TIMEOUT_SECONDS="${HEALTH_TIMEOUT_SECONDS:-120}"
HEALTH_POLL_SECONDS="${HEALTH_POLL_SECONDS:-3}"
DRY_RUN="${DRY_RUN:-false}"

DEPLOY_STARTED_AT="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
ORIGINAL_COMMIT=""
ORIGINAL_BRANCH=""
TARGET_COMMIT=""
IMAGE_TAG=""
TARGET_API_IMAGE=""
TARGET_WEB_IMAGE=""
TARGET_API_IMAGE_ID=""
TARGET_WEB_IMAGE_ID=""
OLD_API_CONTAINER_ID=""
OLD_WEB_CONTAINER_ID=""
OLD_API_IMAGE=""
OLD_WEB_IMAGE=""
OLD_API_IMAGE_ID=""
OLD_WEB_IMAGE_ID=""
BACKUP_PATH=""
TEMP_BACKUP_PATH=""
CHECKOUT_CHANGED=false
DEPLOY_SUCCEEDED=false

log() {
  printf '[%s] %s\n' "$(date -u +'%Y-%m-%dT%H:%M:%SZ')" "$*"
}

error() {
  log "ERROR: $*" >&2
}

critical() {
  log "CRITICAL: $*" >&2
}

on_error() {
  local line="$1"
  local code="$2"
  error "unexpected command failure at line $line (exit $code)"
}

cleanup() {
  if [[ -n "$TEMP_BACKUP_PATH" && -e "$TEMP_BACKUP_PATH" ]]; then
    # A partial backup is never useful; failure to remove it must not hide the deployment result.
    if ! rm -f -- "$TEMP_BACKUP_PATH"; then
      error "could not remove incomplete backup: $TEMP_BACKUP_PATH"
    fi
  fi
}

trap 'on_error "$LINENO" "$?"' ERR
trap cleanup EXIT

require_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    error "required command is not installed: $command_name"
    return 1
  fi
}

require_deploy_ref() {
  if [[ -z "${DEPLOY_REF:-}" ]]; then
    error "DEPLOY_REF is required; use a full commit hash or release tag"
    return 1
  fi
  if [[ ! "$DEPLOY_REF" =~ ^[A-Za-z0-9][A-Za-z0-9._/@:+-]*$ ]]; then
    error "DEPLOY_REF contains unsupported characters"
    return 1
  fi
}

git_status_porcelain() {
  git status --porcelain
}

ensure_clean_worktree() {
  local status
  status="$(git_status_porcelain)" || return 1
  if [[ -n "$status" ]]; then
    error "deployment worktree is not clean; refusing to checkout a deployment ref"
    return 1
  fi
}

load_production_environment() {
  if [[ ! -f "$ENV_FILE" ]]; then
    error "production environment file not found: $ENV_FILE"
    return 1
  fi
  set -a
  # shellcheck disable=SC1090 -- the production path is selected explicitly by ENV_FILE.
  source "$ENV_FILE"
  set +a
}

require_env_value() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    error "$name must be set in $ENV_FILE"
    return 1
  fi
}

require_env_declaration() {
  local name="$1"
  if ! grep -Eq "^[[:space:]]*${name}=" "$ENV_FILE"; then
    error "$name must be explicitly declared in $ENV_FILE"
    return 1
  fi
}

require_boolean_env() {
  local name="$1"
  require_env_value "$name" || return 1
  case "${!name}" in
    true|false) ;;
    *)
      error "$name must be explicitly set to true or false"
      return 1
      ;;
  esac
}

validate_production_environment() {
  local name
  RESERVATION_MODE="${RESERVATION_MODE:-disabled}"
  VITE_RESERVATION_MODE="${VITE_RESERVATION_MODE:-disabled}"
  TRUSTED_PROXY_CIDRS="${TRUSTED_PROXY_CIDRS:-${RESERVATION_TRUSTED_PROXY_CIDRS:-}}"
  for name in APP_ENV JWT_SECRET POSTGRES_USER POSTGRES_PASSWORD POSTGRES_DB PLATFORM_MALL_TENANT_ID VITE_ADMIN_BASE_PATH VITE_API_BASE_URL; do
    require_env_value "$name" || return 1
  done
  for name in OPENAPI_LEGACY_SIGNATURE_ENABLED STOREFRONT_AUTH_ENABLED CUSTOMER_PASSWORD_RESET_ENABLED STOREFRONT_PRICE_VISIBLE VITE_STOREFRONT_AUTH_ENABLED VITE_CUSTOMER_PASSWORD_RESET_ENABLED VITE_STOREFRONT_PRICE_VISIBLE; do
    require_boolean_env "$name" || return 1
  done
  require_env_declaration VITE_MERCHANT_WHATSAPP_URL || return 1
  require_env_value TRUSTED_PROXY_CIDRS || return 1

  if [[ "$APP_ENV" != "production" ]]; then
    error "APP_ENV must be exactly production"
    return 1
  fi
  if (( ${#JWT_SECRET} < 32 )); then
    error "JWT_SECRET is too short; application validation requires at least 32 bytes in production"
    return 1
  fi
  case "${JWT_SECRET,,}" in
    change-me-in-production|dev-secret|secret|your-secret-key|your-very-strong-jwt-secret)
      error "JWT_SECRET uses a known placeholder or insecure default"
      return 1
      ;;
  esac
  if [[ ! "$PLATFORM_MALL_TENANT_ID" =~ ^[1-9][0-9]*$ ]]; then
    error "PLATFORM_MALL_TENANT_ID must be a positive integer"
    return 1
  fi
  if [[ "$VITE_ADMIN_BASE_PATH" != "/gc-control-7x" ]]; then
    error "VITE_ADMIN_BASE_PATH must remain /gc-control-7x for this release"
    return 1
  fi
  if [[ "$VITE_STOREFRONT_AUTH_ENABLED" != "$STOREFRONT_AUTH_ENABLED" ]]; then
    error "frontend and API storefront auth flags do not match"
    return 1
  fi
  if [[ "$VITE_CUSTOMER_PASSWORD_RESET_ENABLED" != "$CUSTOMER_PASSWORD_RESET_ENABLED" ]]; then
    error "frontend and API customer password reset flags do not match"
    return 1
  fi
  if [[ "$CUSTOMER_PASSWORD_RESET_ENABLED" != "false" ]]; then
    error "CUSTOMER_PASSWORD_RESET_ENABLED must remain false until an email delivery provider is implemented"
    return 1
  fi
  if [[ "$VITE_STOREFRONT_PRICE_VISIBLE" != "$STOREFRONT_PRICE_VISIBLE" ]]; then
    error "frontend and API storefront price flags do not match"
    return 1
  fi
  case "$RESERVATION_MODE" in
    disabled|anonymous|otp) ;;
    *)
      error "RESERVATION_MODE must be disabled, anonymous, or otp"
      return 1
      ;;
  esac
  case "$VITE_RESERVATION_MODE" in
    disabled|anonymous|otp) ;;
    *)
      error "VITE_RESERVATION_MODE must be disabled, anonymous, or otp"
      return 1
      ;;
  esac
  if [[ "$VITE_RESERVATION_MODE" != "$RESERVATION_MODE" ]]; then
    error "frontend and API reservation modes do not match"
    return 1
  fi
  if [[ "$RESERVATION_MODE" != disabled ]]; then
    for name in RESERVATION_IP_HASH_SECRET TRUSTED_PROXY_CIDRS TURNSTILE_SITE_KEY TURNSTILE_SECRET_KEY \
      TURNSTILE_EXPECTED_HOSTNAME TURNSTILE_EXPECTED_ACTION VITE_TURNSTILE_SITE_KEY; do
      require_env_value "$name" || return 1
    done
    for name in TURNSTILE_ENABLED VITE_TURNSTILE_ENABLED; do
      require_boolean_env "$name" || return 1
    done

    if [[ "$TURNSTILE_ENABLED" != true || "$VITE_TURNSTILE_ENABLED" != true ]]; then
      error "Turnstile must be enabled in both API and Web production configuration"
      return 1
    fi
    if [[ "$VITE_TURNSTILE_SITE_KEY" != "$TURNSTILE_SITE_KEY" ]]; then
      error "frontend and API Turnstile site keys do not match"
      return 1
    fi
    if (( ${#TURNSTILE_SITE_KEY} < 20 || ${#TURNSTILE_SECRET_KEY} < 20 )); then
      error "Turnstile keys are too short or placeholders"
      return 1
    fi
    case "$TURNSTILE_SITE_KEY" in
      1x00000000000000000000AA|1x00000000000000000000BB|2x00000000000000000000AB|2x00000000000000000000BB|3x00000000000000000000FF)
        error "Cloudflare Turnstile test keys are forbidden in production"
        return 1
        ;;
    esac
    case "$TURNSTILE_SECRET_KEY" in
      1x0000000000000000000000000000000AA|2x0000000000000000000000000000000AA|3x0000000000000000000000000000000AA)
        error "Cloudflare Turnstile test keys are forbidden in production"
        return 1
        ;;
    esac
    if (( ${#RESERVATION_IP_HASH_SECRET} < 32 )); then
      error "reservation IP hash secret is missing or too short"
      return 1
    fi
    if [[ "$TRUSTED_PROXY_CIDRS" == *"0.0.0.0/0"* || "$TRUSTED_PROXY_CIDRS" == *"::/0"* ]]; then
      error "TRUSTED_PROXY_CIDRS must not trust the entire Internet"
      return 1
    fi
    if [[ "$RESERVATION_MODE" == anonymous ]]; then
      for name in RESERVATION_BROWSER_HASH_SECRET RESERVATION_ANONYMOUS_TTL_HOURS \
        RESERVATION_BROWSER_LIMIT_PER_HOUR RESERVATION_BROWSER_LIMIT_PER_DAY \
        RESERVATION_IP_LIMIT_PER_HOUR RESERVATION_IP_LIMIT_PER_DAY RESERVATION_IP_PRODUCT_LIMIT_PER_DAY; do
        require_env_value "$name" || return 1
      done
      if [[ "$TURNSTILE_EXPECTED_ACTION" != reservation_create ]]; then
        error "TURNSTILE_EXPECTED_ACTION must be reservation_create in anonymous mode"
        return 1
      fi
      if (( ${#RESERVATION_BROWSER_HASH_SECRET} < 32 )); then
        error "RESERVATION_BROWSER_HASH_SECRET is missing or too short"
        return 1
      fi
      for name in RESERVATION_ANONYMOUS_TTL_HOURS RESERVATION_BROWSER_LIMIT_PER_HOUR RESERVATION_BROWSER_LIMIT_PER_DAY \
        RESERVATION_IP_LIMIT_PER_HOUR RESERVATION_IP_LIMIT_PER_DAY RESERVATION_IP_PRODUCT_LIMIT_PER_DAY; do
        if [[ ! "${!name}" =~ ^[1-9][0-9]*$ ]]; then
          error "$name must be a positive integer"
          return 1
        fi
      done
    else
      for name in RESERVATION_CONTACT_HASH_SECRET RESERVATION_CONTACT_ENCRYPTION_KEY OTP_PROVIDER RESERVATION_OTP_CHANNELS \
        RESERVATION_OTP_RESPONSE_MIN_MILLISECONDS TWILIO_ACCOUNT_SID TWILIO_AUTH_TOKEN TWILIO_VERIFY_SERVICE_SID; do
        require_env_value "$name" || return 1
      done
      if [[ "$TURNSTILE_EXPECTED_ACTION" != reservation_otp ]]; then
        error "TURNSTILE_EXPECTED_ACTION must be reservation_otp in otp mode"
        return 1
      fi
      if (( ${#RESERVATION_CONTACT_HASH_SECRET} < 32 || ${#RESERVATION_CONTACT_ENCRYPTION_KEY} < 32 )); then
        error "reservation contact/encryption secrets are missing or too short"
        return 1
      fi
      if [[ "$OTP_PROVIDER" != twilio_verify ]]; then
        error "OTP_PROVIDER must be twilio_verify in production"
        return 1
      fi
      if [[ ! "$RESERVATION_OTP_CHANNELS" =~ ^(sms|whatsapp)(,(sms|whatsapp))*$ ]]; then
        error "RESERVATION_OTP_CHANNELS contains an unsupported channel"
        return 1
      fi
      if [[ ! "$RESERVATION_OTP_RESPONSE_MIN_MILLISECONDS" =~ ^[0-9]+$ || "$RESERVATION_OTP_RESPONSE_MIN_MILLISECONDS" -lt 250 || "$RESERVATION_OTP_RESPONSE_MIN_MILLISECONDS" -gt 5000 ]]; then
        error "RESERVATION_OTP_RESPONSE_MIN_MILLISECONDS must be between 250 and 5000 in production"
        return 1
      fi
      if [[ ! "$TWILIO_ACCOUNT_SID" =~ ^AC[0-9A-Fa-f]{32}$ || ! "$TWILIO_VERIFY_SERVICE_SID" =~ ^VA[0-9A-Fa-f]{32}$ || ${#TWILIO_AUTH_TOKEN} -lt 20 ]]; then
        error "Twilio Verify credentials use invalid or placeholder formats"
        return 1
      fi
    fi
  else
    log "reservation feature is disabled; reservation secrets, Turnstile, OTP channels, and Twilio are not required"
  fi
  if [[ ! "$HEALTH_TIMEOUT_SECONDS" =~ ^[1-9][0-9]*$ || ! "$HEALTH_POLL_SECONDS" =~ ^[1-9][0-9]*$ ]]; then
    error "health timeout and poll interval must be positive integers"
    return 1
  fi

  log "production configuration preflight passed (secret values are not printed)"
}

compose() {
  docker compose --env-file "$ENV_FILE" "$@"
}

inspect_container_value() {
  local container_id="$1"
  local format="$2"
  docker inspect --format "$format" "$container_id"
}

capture_current_deployment() {
  log "current running api/web containers:"
  compose ps api web || return 1

  OLD_API_CONTAINER_ID="$(compose ps -q api)" || return 1
  OLD_WEB_CONTAINER_ID="$(compose ps -q web)" || return 1
  if [[ -z "$OLD_API_CONTAINER_ID" || -z "$OLD_WEB_CONTAINER_ID" ]]; then
    error "api and web must both be running before a rollback-safe production deployment"
    return 1
  fi

  OLD_API_IMAGE="$(inspect_container_value "$OLD_API_CONTAINER_ID" '{{.Config.Image}}')" || return 1
  OLD_WEB_IMAGE="$(inspect_container_value "$OLD_WEB_CONTAINER_ID" '{{.Config.Image}}')" || return 1
  OLD_API_IMAGE_ID="$(inspect_container_value "$OLD_API_CONTAINER_ID" '{{.Image}}')" || return 1
  OLD_WEB_IMAGE_ID="$(inspect_container_value "$OLD_WEB_CONTAINER_ID" '{{.Image}}')" || return 1
  log "current api image: $OLD_API_IMAGE ($OLD_API_IMAGE_ID)"
  log "current web image: $OLD_WEB_IMAGE ($OLD_WEB_IMAGE_ID)"
}

resolve_target_commit() {
  log "fetching deployment refs from origin"
  git fetch --tags origin || return 1
  TARGET_COMMIT="$(git rev-parse --verify --end-of-options "${DEPLOY_REF}^{commit}")" || {
    error "DEPLOY_REF does not resolve to a commit: $DEPLOY_REF"
    return 1
  }
  IMAGE_TAG="$(git rev-parse --short=12 "$TARGET_COMMIT")" || return 1
  TARGET_API_IMAGE="${API_IMAGE_REPOSITORY:-saas-api}:$IMAGE_TAG"
  TARGET_WEB_IMAGE="${WEB_IMAGE_REPOSITORY:-saas-web}:$IMAGE_TAG"
  log "current git commit: $ORIGINAL_COMMIT"
  log "target git commit: $TARGET_COMMIT"
  log "target local images: $TARGET_API_IMAGE and $TARGET_WEB_IMAGE"
}

checkout_target_commit() {
  if [[ "$ORIGINAL_COMMIT" == "$TARGET_COMMIT" ]]; then
    return 0
  fi
  git switch --detach "$TARGET_COMMIT" || return 1
  CHECKOUT_CHANGED=true
}

restore_original_checkout() {
  if [[ "$CHECKOUT_CHANGED" != true || -z "$ORIGINAL_COMMIT" ]]; then
    return 0
  fi
  ensure_clean_worktree || {
    critical "cannot restore the original checkout because the worktree became dirty"
    return 1
  }
  if [[ -n "$ORIGINAL_BRANCH" ]]; then
    git switch "$ORIGINAL_BRANCH" || return 1
  else
    git switch --detach "$ORIGINAL_COMMIT" || return 1
  fi
  CHECKOUT_CHANGED=false
  log "restored deployment checkout to $ORIGINAL_COMMIT"
}

write_deployment_state() {
  local file="$1"
  local status="$2"
  mkdir -p "$STATE_DIR" || return 1
  chmod 700 "$STATE_DIR" || return 1
  {
    printf 'STATUS=%q\n' "$status"
    printf 'STARTED_AT=%q\n' "$DEPLOY_STARTED_AT"
    printf 'ORIGINAL_COMMIT=%q\n' "$ORIGINAL_COMMIT"
    printf 'TARGET_COMMIT=%q\n' "$TARGET_COMMIT"
    printf 'IMAGE_TAG=%q\n' "$IMAGE_TAG"
    printf 'OLD_API_IMAGE=%q\n' "$OLD_API_IMAGE"
    printf 'OLD_API_IMAGE_ID=%q\n' "$OLD_API_IMAGE_ID"
    printf 'OLD_API_CONTAINER_ID=%q\n' "$OLD_API_CONTAINER_ID"
    printf 'OLD_WEB_IMAGE=%q\n' "$OLD_WEB_IMAGE"
    printf 'OLD_WEB_IMAGE_ID=%q\n' "$OLD_WEB_IMAGE_ID"
    printf 'OLD_WEB_CONTAINER_ID=%q\n' "$OLD_WEB_CONTAINER_ID"
    printf 'TARGET_API_IMAGE=%q\n' "$TARGET_API_IMAGE"
    printf 'TARGET_API_IMAGE_ID=%q\n' "$TARGET_API_IMAGE_ID"
    printf 'TARGET_WEB_IMAGE=%q\n' "$TARGET_WEB_IMAGE"
    printf 'TARGET_WEB_IMAGE_ID=%q\n' "$TARGET_WEB_IMAGE_ID"
    printf 'BACKUP_PATH=%q\n' "$BACKUP_PATH"
  } >"$file"
  chmod 600 "$file" || return 1
}

production_preflight() {
  log "stage: preflight"
  require_deploy_ref || return 1
  require_command git || return 1
  require_command docker || return 1
  ensure_clean_worktree || return 1

  ORIGINAL_COMMIT="$(git rev-parse HEAD)" || return 1
  if ORIGINAL_BRANCH="$(git symbolic-ref --quiet --short HEAD)"; then
    :
  else
    ORIGINAL_BRANCH=""
  fi

  load_production_environment || return 1
  validate_production_environment || return 1
  capture_current_deployment || return 1
  resolve_target_commit || return 1
  checkout_target_commit || return 1
  write_deployment_state "$STATE_DIR/current-attempt.env" preflight || return 1
}

run_database_invariant_preflight() {
  log "stage: database invariant check ($INVARIANT_SQL)"
  if [[ ! -f "$INVARIANT_SQL" ]]; then
    error "database invariant SQL not found: $INVARIANT_SQL"
    return 1
  fi
  if ! compose exec -T db psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f - <"$INVARIANT_SQL"; then
    error "database invariant preflight failed; no data was modified"
    error "review and repair the read-only findings from $INVARIANT_SQL before retrying"
    return 1
  fi
}

create_database_backup() {
  log "stage: database backup"
  mkdir -p "$BACKUP_DIR" || return 1
  chmod 700 "$BACKUP_DIR" || return 1
  BACKUP_PATH="$BACKUP_DIR/${DEPLOY_STARTED_AT//[:]/-}-${IMAGE_TAG}.dump"
  TEMP_BACKUP_PATH="${BACKUP_PATH}.partial"
  if ! compose exec -T db pg_dump -Fc -U "$POSTGRES_USER" -d "$POSTGRES_DB" >"$TEMP_BACKUP_PATH"; then
    error "pg_dump failed; api/web will not be replaced"
    return 1
  fi
  if [[ ! -s "$TEMP_BACKUP_PATH" ]]; then
    error "pg_dump produced an empty backup; api/web will not be replaced"
    return 1
  fi
  mv "$TEMP_BACKUP_PATH" "$BACKUP_PATH" || return 1
  TEMP_BACKUP_PATH=""
  chmod 600 "$BACKUP_PATH" || return 1
  log "database backup: $BACKUP_PATH"
  log "database backup size: $(wc -c <"$BACKUP_PATH") bytes"
  write_deployment_state "$STATE_DIR/current-attempt.env" backup-complete || return 1
}

build_target_images() {
  log "stage: build target images while current containers remain running"
  export IMAGE_TAG
  export API_IMAGE="$TARGET_API_IMAGE"
  export WEB_IMAGE="$TARGET_WEB_IMAGE"
  compose build api web || {
    error "image build failed; current containers were not replaced"
    return 1
  }
  TARGET_API_IMAGE_ID="$(docker image inspect --format '{{.Id}}' "$TARGET_API_IMAGE")" || return 1
  TARGET_WEB_IMAGE_ID="$(docker image inspect --format '{{.Id}}' "$TARGET_WEB_IMAGE")" || return 1
  log "built api image: $TARGET_API_IMAGE ($TARGET_API_IMAGE_ID)"
  log "built web image: $TARGET_WEB_IMAGE ($TARGET_WEB_IMAGE_ID)"
  write_deployment_state "$STATE_DIR/current-attempt.env" build-complete || return 1
}

replace_application_services() {
  log "stage: replace api/web only (database service and volumes remain running)"
  export IMAGE_TAG
  export API_IMAGE="$TARGET_API_IMAGE"
  export WEB_IMAGE="$TARGET_WEB_IMAGE"
  compose up -d --no-deps --force-recreate api web
}

container_health_status() {
  local service="$1"
  local container_id
  container_id="$(compose ps -q "$service")" || return 1
  if [[ -z "$container_id" ]]; then
    printf 'missing\n'
    return 0
  fi
  inspect_container_value "$container_id" '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}'
}

wait_for_service_health() {
  local service="$1"
  local deadline=$((SECONDS + HEALTH_TIMEOUT_SECONDS))
  local status=""
  while (( SECONDS < deadline )); do
    status="$(container_health_status "$service")" || return 1
    case "$status" in
      healthy)
        log "$service health: healthy"
        return 0
        ;;
      unhealthy|exited|dead)
        error "$service health: $status"
        return 1
        ;;
    esac
    sleep "$HEALTH_POLL_SECONDS"
  done
  error "$service health timed out after ${HEALTH_TIMEOUT_SECONDS}s (last status: ${status:-unknown})"
  return 1
}

wait_for_target_health() {
  log "stage: wait for target health"
  wait_for_service_health api || return 1
  wait_for_service_health web || return 1
}

restore_previous_images() {
  if [[ -z "$OLD_API_IMAGE" || -z "$OLD_WEB_IMAGE" ]]; then
    critical "previous image names were not recorded; application rollback cannot proceed"
    return 1
  fi
  export API_IMAGE="$OLD_API_IMAGE"
  export WEB_IMAGE="$OLD_WEB_IMAGE"
  compose up -d --no-deps --force-recreate api web
}

wait_for_rollback_health() {
  wait_for_service_health api || return 1
  wait_for_service_health web || return 1
}

rollback_application() {
  log "application rollback attempted: restoring $OLD_API_IMAGE and $OLD_WEB_IMAGE"
  if ! restore_previous_images; then
    critical "APPLICATION ROLLBACK FAILED while restoring previous images"
    write_deployment_state "$STATE_DIR/current-attempt.env" rollback-failed || return 1
    return 1
  fi
  if ! wait_for_rollback_health; then
    critical "APPLICATION ROLLBACK HEALTH CHECK FAILED; manual intervention is required immediately"
    write_deployment_state "$STATE_DIR/current-attempt.env" rollback-unhealthy || return 1
    return 1
  fi
  log "application rollback health: restored"
  log "database was not restored automatically; application rollback does not imply schema rollback"
  write_deployment_state "$STATE_DIR/current-attempt.env" rollback-restored || return 1
}

handle_target_failure() {
  error "target deployment failed"
  if ! rollback_application; then
    critical "target deployment and application rollback both failed"
  fi
  return 1
}

record_successful_deployment() {
  log "stage: record successful deployment"
  write_deployment_state "$STATE_DIR/current-attempt.env" successful || return 1
  cp "$STATE_DIR/current-attempt.env" "$STATE_DIR/last-successful.env" || return 1
  chmod 600 "$STATE_DIR/last-successful.env" || return 1
  DEPLOY_SUCCEEDED=true
  log "deployment successful: $TARGET_COMMIT"
  log "database backup retained at: $BACKUP_PATH"
}

execute_deployment() {
  production_preflight || return 1
  run_database_invariant_preflight || return 1
  create_database_backup || return 1
  build_target_images || return 1
  if ! replace_application_services; then
    handle_target_failure
    return 1
  fi
  if ! wait_for_target_health; then
    handle_target_failure
    return 1
  fi
  if ! record_successful_deployment; then
    handle_target_failure
    return 1
  fi
}

run_dry_run() {
  log "DRY_RUN: validating deployment parameters and production configuration only"
  require_deploy_ref || return 1
  require_command git || return 1
  ensure_clean_worktree || return 1
  load_production_environment || return 1
  validate_production_environment || return 1
  log "DRY_RUN complete; no fetch, checkout, database command, image build, or container replacement was executed"
}

main() {
  cd "$PROJECT_DIR" || return 1
  if [[ "$DRY_RUN" == true ]]; then
    run_dry_run
    return
  fi

  if ! execute_deployment; then
    if ! restore_original_checkout; then
      critical "deployment failed and the original git checkout could not be restored"
    fi
    return 1
  fi
}

if [[ "${DEPLOY_LIB_ONLY:-false}" != true ]]; then
  main "$@"
fi
