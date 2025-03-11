#!/bin/sh

# start nginx 
nginx 

# start goproxy
/app/goproxy/bin/goproxy -listen=0.0.0.0:8078 -exclude "pegasus-cloud.com"
