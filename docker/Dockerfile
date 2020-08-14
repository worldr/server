# FROM alpine:3.10
# Because GitHub actions build via that OS, so we cannot use Alpine.
From ubuntu:18.04

# Some ENV variables
ENV PATH="/mattermost/bin:${PATH}"

# Build argument to set Mattermost edition
ARG edition=enterprise
ARG PUID=2000
ARG PGID=2000
ARG MM_BINARY=

# Install some needed packages
#RUN apk add --no-cache \
#	ca-certificates \
#	curl \
#	jq \
#	libc6-compat \
#	libffi-dev \
#    libcap \
#	linux-headers \
#	mailcap \
#	netcat-openbsd \
#	xmlsec-dev \
#	tzdata \
#	&& rm -rf /tmp/*
RUN set -ex \
    && apt-get update \
    && DEBIAN_FRONTEND="noninteractive" apt-get install -y \
	    curl \
	    jq \
	    libffi-dev \
        libcap-dev \
	    linux-headers-generic \
	    mime-support \
	    netcat-openbsd \
	    libxml2-dev \
        libxmlsec1-dev \
        libxmlsec1-openssl \
	    tzdata \
        libcap2-bin

# Get server release tarball. This is safe since GitHub actions are doing it.
COPY dist/mattermost-team-linux-amd64.tar.gz /tmp/latest.tar.gz

# Install the server tarball.
RUN set -ex \
    && tar xvf /tmp/latest.tar.gz --directory / \
    && rm -rf /mattermost/config/config.json \
    && addgroup --gid ${PGID} mattermost \
    && adduser --disabled-login --disabled-password --uid ${PUID} --ingroup mattermost --home /mattermost --no-create-home mattermost \
    && chown -R mattermost:mattermost /mattermost \
    && setcap cap_net_bind_service=+ep /mattermost/bin/mattermost

USER mattermost

#Healthcheck to make sure container is ready
HEALTHCHECK CMD curl --fail http://localhost:8000 || exit 1

# Configure entrypoint and command
COPY docker/entrypoint.sh /
ENTRYPOINT ["/entrypoint.sh"]
WORKDIR /mattermost
CMD ["mattermost"]

# Expose port 8000 of the container
EXPOSE 8000

# Declare volumes for mount point directories
VOLUME ["/mattermost/data", "/mattermost/logs", "/mattermost/config", "/mattermost/plugins", "/mattermost/client/plugins"]
