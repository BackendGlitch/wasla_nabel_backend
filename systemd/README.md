## Systemd units (LAN station deployment)

This folder contains **per-service** systemd units (recommended) plus a **migration** unit.

### Install (example)

- Copy units:

```bash
sudo cp -v /home/ste/wasla_backend/systemd/wasla-*.service /etc/systemd/system/
sudo cp -v /home/ste/wasla_backend/systemd/wasla-backend.target /etc/systemd/system/
```

- Reload + enable target:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now wasla-backend.target
```

### Notes

- **Migrations**: `wasla-migrate.service` runs first and blocks startup on failure.
- **Logs**: by default go to `journalctl -u wasla-booking.service` etc.
- **Environment**: all services read `/home/ste/wasla_backend/configs/environment.env` (edit for server values).

