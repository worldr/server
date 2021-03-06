#!/bin/bash

MAIN_BRANCH=$1
if [ -z "$MAIN_BRANCH" ]; then
	echo Error: main branch name parameter is missing!
	echo Usage: 
	echo tag_main_branch.sh MAIN_BRANCH_NAME
	exit 1
fi

echo INTENDED FOR USE WITH A GITHUB ACTION 
echo TRIGGERED BY A PR MERGE INTO THE MAIN BRANCH \("$MAIN_BRANCH"\)!

BRANCH=$(git symbolic-ref -q --short HEAD)
if [ "$BRANCH" != "$MAIN_BRANCH" ]; then
	echo Error: you are not on the main branch of the repo \("$MAIN_BRANCH"\)!
	exit 2
fi

echo Existing tags:
git fetch --all --tags
git tag

V=$(cat ./VERSION)
EXISTING=$(git tag|grep "$V")
echo Current version is \""$V"\", existing matching version is \""$EXISTING"\".
if [[ -z "$EXISTING" ]]; then
	git tag "v$V"
	git push origin "v$V"
else
	echo Version "$V" is already tagged.
fi