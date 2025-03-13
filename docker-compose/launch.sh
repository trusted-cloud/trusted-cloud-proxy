#!/bin/bash

# check if argument is provided

if [ -z "$1" ]; then
	echo "Please provide your GitHub Personal Access Token"
	exit
fi

# build image
docker build -t ogre0403/goproxy .

# replace <GITHUB_PAT> in vanity.yaml
sed -i '' "s/<GITHUB_PAT>/${1}/" vanity.yaml

docker run -ti --rm \
	--add-host="pegasus-cloud.com:127.0.0.1" \
	--name goproxy \
	-p 8078:8078 \
	-v $(pwd)/vanity.yaml:/etc/vanity.yaml \
	-e GOSUMDB='off' -e GOINSECURE='pegasus-cloud.com' \
	ogre0403/goproxy:latest
