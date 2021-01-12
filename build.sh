#!/usr/bin/env bash

# init swagger doc

swag init


echo  "GOOS=linux go build"

GOOS=linux go build -o solar-graph

export HOST=harbor.cloud2go.cn
export TAG=0.0.3
docker build -t ${HOST}/cloudos-dev/solar-graph:${TAG} .

docker login -p Harbor12345 -u admin ${HOST}

docker push ${HOST}/cloudos-dev/solar-graph:${TAG}

rm -rf solar-graph
