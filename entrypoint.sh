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

# Drop privileges to the datey user and run the application.
exec su-exec datey /app/datey "$@"
