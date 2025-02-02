#!/bin/sh

BRANCH=`git branch | awk '{print $2}'`
BUILD_NUMBER=`tr -dc a-z0-9 </dev/urandom | head -c 13; echo`
DATE=`date +'%Y-%m-%dT%H:%M:%S %Z'`
VERSION="$BUILD_NUMBER built $DATE branch $BRANCH"
echo -n $VERSION > .version
