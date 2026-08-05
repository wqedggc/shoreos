# ShoreOS FIRE 云端安全部署指南

本文只描述生产上线的安全边界和执行前置条件；本次不部署、不迁移真实数据。所有 `<...>` 均为部署者必须替换的占位符，文档不包含真实域名、IP、证书或密钥。

## 生产模型

- 公网入口只有 Nginx：HTTPS 静态托管 Taro H5，`/api/` 反代到 `127.0.0.1:8090`。
- Go 服务只监听 `127.0.0.1:8090`，不直接暴露公网；健康检查也从本机执行。
- MySQL 只允许 loopback 或云内私网地址，禁止公网监听和公网安全组放行。
- `.env`、MySQL 客户端配置、原始账单 storage、备份均为服务器私有文件，权限受限且不进 Git。
- Nginx 模板见 [`deploy/nginx/shoreos-fire.conf`](../deploy/nginx/shoreos-fire.conf)，可复制到 `/etc/nginx/conf.d/` 后按占位符配置。

## 上线前置条件

上线前由用户或运维明确确认以下事项：

1. 已准备 DNS、HTTPS 证书及其私有密钥；证书路径仅写入 Nginx 配置，密钥权限限制为 root 可读。
2. 已准备 Taro H5 的正式构建产物，并将其放入 Nginx 静态 root；不要把构建目录写回本仓库。
3. 云安全组只放行 `80/443`（以及受限的 SSH 管理入口）；不放行 `8090/3306`。
4. 服务器工作树通过既定的 CNB 只读拉取链更新；GitHub 是主仓，镜像流程见 [`docs/cnb_mirror.md`](cnb_mirror.md)。不要在服务器保存写入凭据。
5. 已人工确认 schema 版本、备份策略和数据迁移窗口。`deploy.sh` 会执行 `schema/mysql/001_shoreos_fire.sql`，因此不能把它当作“无迁移”的生产上线命令；schema/真实数据迁移属于用户显式后续步骤。

## 服务与数据库边界

生产 `.env` 至少应保持以下方向（值由部署者在服务器填写）：

```dotenv
SHOREOS_HTTP_ADDR=127.0.0.1:8090
SHOREOS_MYSQL_HOST=127.0.0.1
SHOREOS_MYSQL_PORT=3306
SHOREOS_ENABLE_LOCAL_BOOTSTRAP=false
SHOREOS_LEDGER_STORAGE_DIR=storage/ledger
```

创建或修改后立即执行 `chmod 600 .env`。MySQL 用户应采用最小权限账号；若使用私网数据库，`SHOREOS_MYSQL_HOST` 只能填写已确认的私网地址，并通过私网 ACL 限制来源。

原始账单、导入附件、数据库导出和备份必须位于 Git 工作树之外或已由 `.gitignore` 明确排除的私有目录，并设置为仅服务账号可读写。上线前检查 `git status`、Git 忽略规则和备份目录，确认没有原始账单或密钥。

## 首次生产 bootstrap（仅本机回环）

首次初始化必须通过 SSH 登录服务器后，直接在服务器本机向 `127.0.0.1:8090` 执行 curl；不可经 Nginx 或从公网浏览器调用。Nginx 永远拒绝 `/api/v1/auth/bootstrap`，避免反代后的 loopback 来源绕过本机初始化边界：

```bash
ssh <ssh-user>@<server-host>
cd <server-app-dir>
chmod 600 .env
# 仅首次启动前临时设置，并确认管理员密码已写入 .env
sed -i.bak 's/^SHOREOS_ENABLE_LOCAL_BOOTSTRAP=.*/SHOREOS_ENABLE_LOCAL_BOOTSTRAP=true/' .env
<service-start-command>
curl --fail --silent --show-error -X POST http://127.0.0.1:8090/api/v1/auth/bootstrap
```

确认返回成功后，立即关闭 bootstrap 并重启服务：

```bash
sed -i.bak 's/^SHOREOS_ENABLE_LOCAL_BOOTSTRAP=.*/SHOREOS_ENABLE_LOCAL_BOOTSTRAP=false/' .env
chmod 600 .env
<service-restart-command>
curl --fail --silent --show-error http://127.0.0.1:8090/healthz
curl --fail --silent --show-error http://127.0.0.1:8090/readyz
```

如果返回 `ALREADY_INITIALIZED`，停止重复初始化并改走正常登录。服务端仍执行 loopback 校验；bootstrap 完成后配置必须恢复为 `false`。Nginx 对该路径保持精确 `404`，后续也不得改回通用 `/api/` 反代。

## Nginx 安装与验证（静态步骤）

将模板复制到 `/etc/nginx/conf.d/`，替换域名、静态 root 和证书占位符；确认 `root` 是正式 Taro H5 构建目录。先执行 `nginx -t`，通过后再由运维系统 reload。若机器未安装 Nginx，`nginx -t` 无法执行，本次仅能完成文本审查，不能宣称配置已验证。

外部验证只应访问 HTTPS 域名；本机验证 Go 服务时使用 `127.0.0.1:8090`。公网不应能访问 `:8090`、`:3306` 或 storage/备份路径。

## 数据迁移边界

本指南不执行 schema 导入、账单导入、数据库恢复或任何真实数据迁移。迁移必须作为单独、经用户显式确认的后续变更，先完成备份、回滚方案、权限审查和小样本校验，再在维护窗口执行。

详细核对项见 [`docs/cloud-security-checklist.md`](cloud-security-checklist.md)。
