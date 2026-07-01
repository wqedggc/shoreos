# GitHub 主仓 + CNB 国内镜像

## 目标

- GitHub `https://github.com/wqedggc/shoreos` 是唯一主仓。
- CNB `https://cnb.cool/wqedggc/shoreos` 只做国内部署镜像。
- 腾讯云 CVM 只从 CNB 拉代码，不访问 GitHub。
- 服务器只保存 CNB 只读拉取凭据；CNB 写入凭据只放在 GitHub Actions Secret。

## GitHub Actions Secret

在 GitHub 仓库 `Settings -> Secrets and variables -> Actions` 增加：

```text
CNB_PUSH_URL=https://<cnb-user-or-token>:<repo-write-token>@cnb.cool/wqedggc/shoreos.git
```

要求：

- token 只给 `wqedggc/shoreos` 这个 CNB 仓库写权限。
- 不使用个人长期高权限 token。
- 不把 token 写入 `.env`、remote URL、文档或 shell history。

配置后，每次 push GitHub `main`，`.github/workflows/mirror-cnb.yml` 会自动执行：

```bash
git push cnb HEAD:main
```

如果还没配置 `CNB_PUSH_URL`，workflow 会跳过 mirror，并在 Actions 日志里输出 notice。

如果 CNB 上有人直接提交导致历史分叉，mirror 会失败。这是预期行为：CNB 是镜像，不应该直接改。

## 腾讯云服务器 Remote

服务器目录：

```bash
cd /home/work/shoreos-fire-server
```

推荐使用 CNB 只读凭据设置 remote：

```bash
git remote set-url origin https://<cnb-readonly-user-or-token>:<repo-read-token>@cnb.cool/wqedggc/shoreos.git
```

如果 CNB 支持 Deploy Key，优先用只读 Deploy Key：

```bash
git remote set-url origin git@cnb.cool:wqedggc/shoreos.git
```

服务器更新流程：

```bash
cd /home/work/shoreos-fire-server
git pull --ff-only origin main
./deploy.sh
```

## 验证

GitHub Actions 成功后，服务器验证：

```bash
cd /home/work/shoreos-fire-server
git fetch origin main
git rev-parse origin/main
curl http://127.0.0.1:8090/healthz
```
