FROM alpine:3.10

# Some ENV variables
ENV PATH="/mattermost/bin:${PATH}"
ENV MM_VERSION=5.23.0
ENV WORLDR_SERVER_VERSION=1.1.0
ENV WORLDR_SERVER_MD5=93f16c9ab12bf336191db2687e69210f
ENV WORLDR_SERVER_URL=https://github.com/worldr/server/releases/download
ENV WORLDR_SERVER_TARBALL=mattermost-team-linux-amd64.tar.gz

# Build argument to set Mattermost edition
ARG edition=enterprise
ARG PUID=2000
ARG PGID=2000
ARG MM_BINARY=


# Install some needed packages
RUN apk add --no-cache \
	ca-certificates \
	curl \
	jq \
	libc6-compat \
	libffi-dev \
    libcap \
	linux-headers \
	mailcap \
	netcat-openbsd \
	xmlsec-dev \
	tzdata \
	&& rm -rf /tmp/*

# Get server release tarball.
RUN set -ex \
    && mkdir -p /mattermost /mattermost/data /mattermost/plugins /mattermost/client/plugins \
    && curl -L $WORLDR_SERVER_URL/v$WORLDR_SERVER_VERSION/$WORLDR_SERVER_TARBALL --output /tmp/latest.tar.gz \
    && echo "${WORLDR_SERVER_MD5}  /tmp/latest.tar.gz" | md5sum -c \
    && tar xvf /tmp/latest.tar.gz --directory / \
    && rm -rf /mattermost/config/config.json \
    && addgroup -g ${PGID} mattermost \
    && adduser -D -u ${PUID} -G mattermost -h /mattermost -D mattermost \
    && chown -R mattermost:mattermost /mattermost \
    && setcap cap_net_bind_service=+ep /mattermost/bin/mattermost

USER mattermost

#Healthcheck to make sure container is ready
HEALTHCHECK CMD curl --fail http://localhost:8000 || exit 1

# Configure entrypoint and command
COPY entrypoint.sh /
ENTRYPOINT ["/entrypoint.sh"]
WORKDIR /mattermost
CMD ["mattermost"]

# Expose port 8000 of the container
EXPOSE 8000

# Declare volumes for mount point directories
VOLUME ["/mattermost/data", "/mattermost/logs", "/mattermost/config", "/mattermost/plugins", "/mattermost/client/plugins"]
