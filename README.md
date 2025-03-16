# Trusted Cloud Proxy

透過 Go module proxy 與 Vanity URL 下載 Host在Github 上的 可信賴雲 dependency package。

## Prepare the environment

```shell
echo "127.0.0.1     proxy.pegasus-cloud.com" >> /etc/hosts
```


## Launch Proxy
```
cd docker-compose
./launch.sh <GIHUB_PAT>

```