# Worldr Server

[![CircleCI](https://circleci.com/gh/worldr/server.svg?style=shield&circle-token=66990f08c761df247eafc0a19fc2f975ffed14a6)](https://app.circleci.com/pipelines/github/worldr/server)

## PostgreSQL transparent data encryption

We use the [CYBERTEC TDE patched version of PostgreSQL](https://www.cybertec-postgresql.com/en/products/postgresql-transparent-data-encryption/).

This means we are building our own Alpine docker image for PostgreSQL. To get
it, you need to run the following before starting the sever.

```bash
docker login -p ${DOCKER_REGISTRY_PASSWORD} -u ${DOCKER_REGISTRY_USERNAME} ${DOCKER_REGISTRY_HOSTNAME}
```

Where `DOCKER_REGISTRY_PASSWORD`, `DOCKER_REGISTRY_USERNAME`, and
`DOCKER_REGISTRY_HOSTNAME` are available in the usual place™.

Note that you might have to run `make clean-docker` to trash all the old
docker images and the old database store.

You can view the images currently in the registry via:

```bash
docker image ls ${DOCKER_REGISTRY_HOSTNAME}/worldr-postgres-tde
```

## Versioning

bump2version is used to update the version of the project:

```bash
bumpversion patch
# OR
bumpversion minor
# OR
bumpversion major
```

This changes two files in the project root: `VERSION.txt` and `.bumpversion.cfg`.

When the version is bumped and the PR with these changes gets merged into the main branch, the `tag_main_branch.sh` gets executed automatically to create a matching git tag. This results in the CI procedures getting executed for the new tagged release.

The Github action being triggered is `.github/workflows/auto-version-tag.yml`.

## Populating server with sample data

The `populate` command creates users and data on the server. This is necessary for testing on the dev server. 

The `--configuration-file` parameter tells the command where to get a data file with users and channels.

The format is as follows:
```json
{
    "open channels names": [
		"General"
    ],
    "team channels names": [
        "Worldr Technologies Ltd"
    ],
    "work channels names": [
        "FFF testing"
    ],
    "personal channels names": [
    ],
    "administrators": [
        {
		    "biography": "",
	        "channel display mode": "",
	        "collapse previews": "",
	        "email": "",
	        "first name": "",
	        "last name": "",
	        "location": "",
	        "message display": "",
	        "nickname": "",
	        "phone number": "",
	        "position": "",
	        "system roles": "",
		    "social media": "",
	        "tutorial step": "",
	        "use military time": "",
	        "username": ""
        }
    ],
	"users": [
        {
            "biography": "",
	        "channel display mode": "",
	        "collapse previews": "",
	        "email": "",
	        "first name": "",
	        "last name": "",
	        "location": "",
	        "message display": "",
	        "nickname": "",
	        "phone number": "",
	        "position": "",
	        "system roles": "",
		    "social media": "",
	        "tutorial step": "",
	        "use military time": "",
	        "username": ""
        }		
	]
}
```

The `--users 10` parameter tells the command how many random users to create. This can be zero if only the data file must be used for population.

The command creates users and conversation between them.

```[[bash]]
./bin/mattermost populate \
--seed 1 \
--users 10 \
--posts-per-channel 100 \
--configuration-file /Users/nox/Documents/workspace/worldr/population/population-testing.json \
--profile-images /Users/nox/Documents/workspace/worldr/population/avatars \
--channel-images /Users/nox/Documents/workspace/worldr/population/channels
```
