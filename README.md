# Trusted Cloud Proxy

透過 Go module proxy 與 Vanity URL 下載 Host在Github 上的 可信賴雲 dependency package。

## Prepare the environment

```shell
echo "127.0.0.1     proxy.pegasus-cloud.com" >> /etc/hosts
```


## Launch Proxy
```shell
# build image
make release-image

# run proxy with valid PAT
make REPO_TOKEN=<YOUR-GITHUB-PAT>  run-proxy
```


## Equivalent GIT CLI for Go module proxy

### list

```bash
git ls-remote --tags https://dummy:${GITLAB_TOKEN}@git.narl.org.tw/gitlab-ee/trusted-cloud/services/toolkits.git | rev | cut -d/ -f1 | rev
git ls-remote --tags https://dummy:${GITHUB_TOKEN}@github.com/trusted-cloud/toolkits.git | rev | cut -d/ -f1 | rev
```


### mode

```bash
git clone -b v0.4.5 https://dummy:${GITLAB_TOKEN}@git.narl.org.tw/gitlab-ee/trusted-cloud/services/toolkits.git

git clone -b v0.4.5 https://dummy:${GITHUB_TOKEN}@github.com/trusted-cloud/toolkits.git 
```


### info

```bash
GIT_PAGER=cat git log -1 --format=%cI
```

### zip

```bash
git archive --prefix=pegasus-cloud.com/aes/toolkits@v0.4.5/ --format zip --output source.zip v0.4.5 . ':!/.git*'
```