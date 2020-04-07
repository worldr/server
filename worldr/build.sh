#!/bin/bash

rm -rf ./plugins/com.worldr.main
set -e
go build -v -o ./plugins/com.worldr.main/plugin ./worldr/com.worldr.main/plugin.go
cp ./worldr/com.worldr.main/plugin.json ./plugins/com.worldr.main/
cd ./plugins/com.worldr.main/
tar -czvf worldr.tar.gz plugin plugin.json