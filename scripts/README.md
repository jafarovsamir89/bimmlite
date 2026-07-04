# BimmLite Deploy Scripts

## Local update

Run from Windows:

```powershell
.\scripts\update.ps1
```

What it does:

1. Pushes `main` to `origin`.
2. SSHes into `free-server-forever`.
3. Runs `bash ~/bimmlite/scripts/deploy.sh` on the server.

## Server deploy

Run on the VM:

```bash
bash ~/bimmlite/scripts/deploy.sh
```

## Environment variables

- `APP_DIR` - repo path on the server, default `~/bimmlite`
- `BRANCH` - branch to deploy, default `main`
- `SERVICE` - systemd service name, default `bimmlite-backend`
- `NGINX_ROOT` - override for the static root if auto-detection fails
- `LOG_FILE` - deploy log path, default `/var/log/bimmlite-deploy.log`

## Logging

- Deploy log: `/var/log/bimmlite-deploy.log`
- Backend service log: `journalctl -u bimmlite-backend.service`

## Notes

- The script never touches `backend/.env`.
- `backend/data/DTC.dat` is preserved.
- Static files are replaced only inside the nginx document root.
