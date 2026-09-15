#!/bin/sh
set -e

# Ensure the data directory exists and is writable by the datey user.
# This is needed because bind-mounted host directories may have incorrect
# ownership/permissions at runtime, regardless of the Dockerfile's RUN mkdir.
if [ ! -d "$DATA_DIR" ]; then
    mkdir -p "$DATA_DIR"
fi
chown -R datey:datey "$DATA_DIR"

# If a media directory is mounted, ensure it is writable too.
if [ -d /app/media ]; then
    chown -R datey:datey /app/media
fi

# Read the OIDC secret as root (the file is typically 0600 root:root) and pass
# it via env, since the datey user cannot read it after dropping privileges.
if [ -n "$OIDC_CLIENT_SECRET_FILE" ] && [ -r "$OIDC_CLIENT_SECRET_FILE" ]; then
    OIDC_CLIENT_SECRET=$(cat "$OIDC_CLIENT_SECRET_FILE")
    export OIDC_CLIENT_SECRET
    unset OIDC_CLIENT_SECRET_FILE
fi

# Drop privileges to the datey user and run the application.
exec su-exec datey /app/datey "$@"
