#!/usr/bin/env bash

docker buildx build --platform linux/amd64,linux/arm64 -f firebase.Dockerfile -t jitsucom/source-firebase:0.0.3 --push .
