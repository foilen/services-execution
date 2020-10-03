# About

This is an application that runs as "root" and that executes multiples applications.

All their outputs are in the main stdout/stderr.

When any application stops, everything stops.

# Local Usage

## Compile

`./create-local-release.sh`

The file is then in `build/bin/services-execution`

## Execute

To execute:
`./build/bin/services-execution config.json`

# Create release

`./create-public-release.sh`

That will show the latest created version. Then, you can choose one and execute:
`./create-public-release.sh X.X.X`

# Use with debian

```bash
echo "deb https://dl.bintray.com/foilen/debian stable main" | sudo tee /etc/apt/sources.list.d/foilen.list
sudo apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys 379CE192D401AB61
sudo apt update
sudo apt install services-execution
```

# Configuration file

Content of config.json:
```
{
    "services": [
        {
            "userID": 70013,
            "groupID": 70013,
            "workingDirectory": "/tmp/",
            "command": "/usr/bin/sleep 987h"
        },
        {
            "userID": 1000,
            "groupID": 1000,
            "workingDirectory": "/",
            "command": "/usr/bin/sleep 567h"
        },
        {
            "userID": 0,
            "groupID": 0,
            "workingDirectory": "/",
            "command": "/usr/bin/sleep 10s"
        }
    ]
}
```

With this example, all the apps will stop in 10 seconds.
