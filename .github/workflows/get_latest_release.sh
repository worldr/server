#!/bin/bash

set -e

GITHUB_TOKEN=$1
REPO=$2
FOLDER=$3

ARGS=0;
if [[ -z "$GITHUB_TOKEN" ]]; then
	ARGS=1;
fi
if [[ -z "$REPO" ]]; then
	ARGS=2;
fi
if [[ -z "$FOLDER" ]]; then
	ARGS=3;
fi
if [[ $ARGS != 0 ]]; then 
	echo Error: parameters are missing!
	echo Usage:
	echo get_latest_release.sh GITHUB_TOKEN REPO_NAME TARGET_FOLDER
	exit 1;
fi

echo INTENDED FOR USE WITH A GITHUB ACTION 
echo TRIGGERED BY PUBLISHING A GITHUB RELEASE!

TARGET="$FOLDER/release.tar.gz"

mkdir -p "$FOLDER";

echo "Using Authorization: Bearer with a github token provided by Github action"
echo "To debug with a personal token use Authorization: Token"

latest=$(curl -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/vnd.github.v3+json" "https://api.github.com/repos/worldr/$REPO/releases/latest");
tarball=$(echo $latest|grep -Eo 'https://api.github.com/repos/worldr/'$REPO'/releases/assets/[[:digit:]]+'); 

echo "Downloading release: $tarball"

curl -L -o $TARGET \
	-H "Authorization: Bearer $GITHUB_TOKEN" \
	-H "Accept: application/octet-stream" \
	$tarball; 

echo "Latest release was saved to $TARGET"
