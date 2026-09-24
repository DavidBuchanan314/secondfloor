# secondfloor

Converts a certain proprietary music collection format into open standards, with support for continuous automatic synchronization and metadata tagging.

```sh
go install github.com/DavidBuchanan314/secondfloor/cmd/secondfloor@latest
export SECONDFLOOR_HMAC_SECRET="..."  # a human-readable string that you need to provide
secondfloor sync ~/Music/
```

Tested on desktop Linux x86-64. May work on other platforms too, ymmv.

## Enabling background service

```sh
cp contrib/secondfloor.service ~/.config/systemd/user/
# edit the .service file to specify SECONDFLOOR_HMAC_SECRET
systemctl --user daemon-reload
systemctl --user enable --now secondfloor
```

By default, this will continuously sync any new downloads into `~/Music/` (configurable), using fsnotify to detect changes.
