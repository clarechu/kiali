#!/usr/bin/env bash

# init swagger doc

swag init


echo  "GOOS=linux go build"

GOOS=linux go build -o solar-graph

export HOST=harbor.cloud2go.cn
export TAG=v1.4.1
docker build -t ${HOST}/solarmesh/solar-graph:${TAG} .

docker login -p Harbor12345 -u admin ${HOST}

docker push ${HOST}/solarmesh/solar-graph:${TAG}

rm -rf solar-graph

kubectl delete po -l app=kiali -n service-mesh