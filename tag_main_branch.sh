#!/bin/bash

MAIN_BRANCH=$1
if [ -z "$MAIN_BRANCH" ]; then
	echo Usage: tag_main_branch MAIN_BRANCH_NAME
	echo Error: main branch name parameter is missing!
	exit	
fi

echo INDENTED FOR USE WITH A GITHUB ACTION 
echo TRIGGERED BY A PR MERGE INTO THE MAIN BRANCH \("$MAIN_BRANCH"\)!

BRANCH=$(git symbolic-ref -q --short HEAD)
if [ "$BRANCH" != "$MAIN_BRANCH" ]; then
	echo Error: you are not on the main branch of the repo \("$MAIN_BRANCH"\)!
	exit
fi

V=$(cat ./VERSION.txt)
EXISTS=$(git tag|grep "$V")
if [[ -z "$EXISTS" ]]; then
	git tag "v$V"
	git push origin "v$V"
else
	echo Version "$V" is already tagged.
fi