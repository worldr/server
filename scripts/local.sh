#!/bin/bash

pg_dump --schema-only -U mmuser -W -d mattermost_test -h 127.0.0.1 > ./local_dump.sql