#!/bin/sh

BRANCH=`git branch | awk '{print $2}'`
BUILD_NUMBER=`git rev-parse HEAD`
DATE=`date +'%Y-%m-%dT%H:%M:%S %Z'`
VERSION="$BUILD_NUMBER::$DATE::$BRANCH"
echo -n $VERSION > .version
