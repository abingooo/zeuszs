# ZEUSZS host updater

`zeuszs-updater` is a root-owned helper for the ZEUSZS test deployment. It only listens on a Unix socket and only accepts an authenticated release tag. The requested tag cannot override commands, paths, Compose services, database targets, or health endpoints.

The updater downloads release assets from `abingooo/zeuszs`, verifies every downloaded file against `checksums-linux.txt`, backs up the Compose files, `.env`, and PostgreSQL database, builds `zeuszs:<version>`, and switches `zeuszs:stable`. It recreates only the `new-api` service. PostgreSQL, Redis, nginx, and sub2api are never recreated. Failed health or version verification restores the previous stable image, recreates the app service, and waits for the old container to become healthy.

## Install

1. Download the architecture-matched `zeuszs-updater-linux-<arch>-<tag>` and `checksums-linux.txt` from the GitHub Release, then verify the SHA256 entry before installation.
2. Install the binary as `/usr/local/sbin/zeuszs-updater` with owner `root:root` and mode `0755`.
3. Install `zeuszs-updater.service` in `/etc/systemd/system/`.
4. Create `/etc/zeuszs-updater.env` from `zeuszs-updater.env.example`, set a long random token, and set mode `0600`.
5. Create `/opt/zeuszs/releases` and `/opt/zeuszs/backups` with owner `root:root` and mode `0700`.
6. Before the first managed update, tag the currently deployed, verified version as `zeuszs:stable` (for example, `docker tag zeuszs:v0.4.0 zeuszs:stable`). Ensure the ordered main and override Compose files resolve only the `new-api` service to `image: zeuszs:stable`.
7. Mount the complete `/run/zeuszs-updater` host directory at `/run/zeuszs-updater` read-only in the `new-api` container and provide the same updater token to the application through its protected runtime secret configuration.
8. Run `systemctl daemon-reload` and `systemctl enable --now zeuszs-updater`.

The supplied systemd unit sets `DOCKER_CONFIG=/var/lib/zeuszs-updater/docker-config`.
This keeps Docker's client configuration in the updater state directory, which
is writable by the service even with `ProtectHome=true` and `ProtectSystem=strict`.

The complete runtime directory is mounted instead of the socket file so a helper restart cannot leave the application bound to a stale Unix-socket inode. Do not commit the real token or place it in the Compose file.

The updater refuses prerelease, replay, and downgrade requests. It also verifies that the configured Compose service resolves to exactly `zeuszs:stable`, enforces download size limits, and requires the configured minimum free space on the release, backup, and Docker filesystems before it changes an image tag.

Automatic rollback restores only the previous application image. The PostgreSQL dump is a disaster-recovery artifact for manual restoration; restoring it automatically could discard writes accepted after the backup was created.

## Local API check

The platform backend should call the socket directly. For an operator-only diagnostic on the host:

```sh
curl --unix-socket /run/zeuszs-updater/updater.sock \
  -H 'Authorization: Bearer <token>' \
  http://localhost/v1/status
```

Status and log files are stored under `/var/lib/zeuszs-updater`. Release files live under `/opt/zeuszs/releases/<tag>`, and timestamped Compose/PostgreSQL backups live under `/opt/zeuszs/backups`.
