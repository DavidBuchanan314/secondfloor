# secondfloor

Converts a certain proprietary music collection format into open standards, with support for continuous automatic synchronization and metadata tagging.

```sh
go install github.com/DavidBuchanan314/secondfloor/cmd/secondfloor@latest
export SECONDFLOOR_HMAC_SECRET="..."  # a human-readable string that you need to provide
secondfloor sync ~/Music/
```

Tested on desktop Linux x86-64. May work on other platforms too, ymmv.

## Windows

```powershell
go install github.com/DavidBuchanan314/secondfloor/cmd/secondfloor@latest
setx SECONDFLOOR_HMAC_SECRET "..."   # takes effect in new terminals
secondfloor sync $HOME\Music
```

While Spotify runs it keeps `index.dat` and the metadata database's `MANIFEST` and journal open without sharing them. secondfloor reads `index.dat` through Spotify's own handle and rebuilds the manifest from the database's table files, so `sync` and `-watch` work with Spotify open. Metadata that Spotify has so far only written to its journal is missed until it lands in a table. Output names avoid Windows reserved names (`CON`, `NUL`, ...) and trailing dots.

## Enabling background task (Windows)

```powershell
$exe = (Get-Command secondfloor).Source
$log = "$env:LOCALAPPDATA\secondfloor.log"
$action = New-ScheduledTaskAction -Execute conhost.exe -Argument ('--headless "{0}" sync -watch -log "{1}" "{2}"' -f $exe, $log, "$HOME\Music")
$trigger = New-ScheduledTaskTrigger -AtLogOn -User "$env:USERDOMAIN\$env:USERNAME"
$settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit 0 -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
Register-ScheduledTask secondfloor -Action $action -Trigger $trigger -Settings $settings
Start-ScheduledTask secondfloor
```

This runs `sync -watch` at logon without a window and logs to `%LOCALAPPDATA%\secondfloor.log`. Stopping the task ends `conhost` but not secondfloor, so to remove it run `Unregister-ScheduledTask secondfloor` and then `Stop-Process -Name secondfloor`.

## Enabling background service (Linux)

```sh
cp contrib/secondfloor.service ~/.config/systemd/user/
# edit the .service file to specify SECONDFLOOR_HMAC_SECRET
systemctl --user daemon-reload
systemctl --user enable --now secondfloor
```

By default, this will continuously sync any new downloads into `~/Music/` (configurable), using fsnotify to detect changes.
