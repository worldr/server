#!/bin/bash

MAIN_BRANCH=$1
if [ -z "$MAIN_BRANCH" ]; then
	echo Usage: tag_main_branch MAIN_BRANCH_NAME
	echo Error: main branch name parameter is missing!
	exit 1
fi

echo INTENDED FOR USE WITH A GITHUB ACTION 
echo TRIGGERED BY A PR MERGE INTO THE MAIN BRANCH \("$MAIN_BRANCH"\)!

BRANCH=$(git symbolic-ref -q --short HEAD)
if [ "$BRANCH" != "$MAIN_BRANCH" ]; then
	echo Error: you are not on the main branch of the repo \("$MAIN_BRANCH"\)!
	exit 2
fi

V=$(cat ./VERSION.txt)
EXISTING=$(git tag|grep "$V")
echo Current version is "$V", existing version is "$EXISTING".
if [[ -z "$EXISTING" ]]; then
	git tag "v$V"
	git push origin "v$V"
else
	echo Version "$V" is already tagged.
fi