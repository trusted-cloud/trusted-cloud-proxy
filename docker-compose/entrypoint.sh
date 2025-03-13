#!/bin/sh

# start nginx
echo "Start nginx daemon"
nginx

# start govanityurls
echo "Start govanityurls in background"
/usr/local/bin/govanityurls /etc/vanity.yaml &

# start goproxy
#/usr/local/bin/goproxy -listen=0.0.0.0:8078 -cacheDir="/tmp/download/module" -exclude "pegasus-cloud.com"
echo "Start goproxy"
/usr/local/bin/goproxy -listen=0.0.0.0:8078 -cacheDir="/tmp/download/module" -exclude ${GOINSECURE}
