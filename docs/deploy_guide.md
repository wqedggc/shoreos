# ShoreOS FIRE 部署流程

## 目标

- 云厂商：腾讯云 CVM。
- 访问方式：一期 IP 直连，默认 `http://<服务器IP>:8090/`。
- 服务器目录：`/home/work/shoreos-fire-server/`。
- 数据库：MySQL `shoreos`。
- 前端：`web/static/` 通过 Go `//go:embed` 嵌入二进制。

## 仓库

- GitHub：`https://github.com/wqedggc/shoreos.git`
- 腾讯侧镜像推荐：`https://cnb.cool/wqedggc/shoreos`

本地路径：

```bash
/Users/shore/Desktop/ShoreOS/services/shoreos-fire-server
```

## 首次部署

```bash
ssh -i ~/shore.pem root@43.143.208.153

cd /home/work
git clone https://cnb.cool/wqedggc/shoreos.git shoreos-fire-server
cd shoreos-fire-server

cp .env.example .env
vi .env

chmod +x deploy.sh
./deploy.sh
```

## 日常更新

本地：

```bash
cd /Users/shore/Desktop/ShoreOS/services/shoreos-fire-server
git add -A
git commit -m "feat: update shoreos fire"
git push cnb main
/Users/shore/Desktop/Knowledge/tools/github_device_push.sh \
  --repo /Users/shore/Desktop/ShoreOS/services/shoreos-fire-server \
  --remote origin \
  --ref main
```

服务器：

```bash
ssh -i ~/shore.pem root@43.143.208.153
cd /home/work/shoreos-fire-server
git pull cnb main
./deploy.sh
```

## 验证

```bash
curl http://127.0.0.1:8090/healthz
curl http://127.0.0.1:8090/readyz
curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8090/
```

浏览器访问：

```text
http://<服务器IP>:8090/
```
