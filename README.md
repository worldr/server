# Worldr Server


[![CircleCI](https://circleci.com/gh/worldr/server.svg?style=shield&circle-token=66990f08c761df247eafc0a19fc2f975ffed14a6)](https://app.circleci.com/pipelines/github/worldr/server)

# Populating server with sample data

```bash
# run in the server directory
./bin/mattermost populatesample --admins "nox,max,hans,yann,sean,dz,tanel"
```

```bash
./bin/mattermost populatesample --help

Generate sample data

Usage:
  mattermost populatesample [flags]

Flags:
      --admins strings          Server admins.
      --deactivated-users int   The number of deactivated users.
  -g, --guests int              The number of sample guests.
  -h, --help                    help for populatesample
      --posts-per-channel int   The number of sample post per channel. (default 40)
      --profile-images string   Optional. Path to folder with images to randomly pick as user profile image.
      --channel-images string   Optional. Path to folder with images to randomly pick as channel image.			
  -s, --seed int                Seed used for generating the random data (Different seeds generate different data). (default 3)
  -u, --users int               The number of sample users. (default 15)

Global Flags:
  -c, --config string        Configuration file to use. (default "config.json")
      --disableconfigwatch   When set config.json will not be loaded from disk when the file is changed.
```