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

## Populating server with sample data

There is an extra command `populate` that will populate the server with either
random data or data taken from a configuration file.

Useful commands:

```[[bash]]
$ ./bin/mattermost populate --help
$ ./bin/mattermost populate --configuration-file /path/to/file.json
$ ./bin/mattermost populatesample --admins "nox,max,hans,yann,sean,dz,tanel"
```
