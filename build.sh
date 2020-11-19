#!/usr/bin/env bash

# init swagger doc

swag init


echo  "GOOS=linux go build"

GOOS=linux go build -o kiali

export HOST=harbor.cloud2go.cn
export TAG=0.0.1
docker build -t ${HOST}/cloudos-dev/kiali:${TAG} .

docker login -p Harbor12345 -u admin ${HOST}

docker push ${HOST}/cloudos-dev/kiali:${TAG}

rm -rf manager
