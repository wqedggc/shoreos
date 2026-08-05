# 通用身份能力与人类可读账单

## 边界

ShoreOS 是模块化单体：身份、账本与 FIRE 共用同一个后端、会话和用户 ID。

- 念黄只复用 bcrypt、Bearer 会话和微信 code 交换的实现思路，不共用用户库、角色或设备模型。
- 业务接口从 Bearer 会话取得可信 `userID`；请求体和查询参数不能指定数据归属用户。
- 原始流水、原件和数据库凭据均在本机私有存储中，不进入 Git。

## 身份流程

1. 首次本机初始化仅在 `SHOREOS_ENABLE_LOCAL_BOOTSTRAP=true`、请求来自 loopback 且用户表为空时可用。
2. 账号密码使用 bcrypt；已存在的旧 SHA-256 密码只会在一次成功登录后升级。
3. 小程序调用 `wx.login` 取得 code，服务端换取 OpenID，已绑定用户才会获得 ShoreOS 会话。
4. H5 已登录用户创建一次性绑定码；小程序凭绑定码和 code 绑定 OpenID。绑定码默认十分钟有效且只能消费一次。

## 账本数据流

```text
私有账单文件
  -> ledger_v2_source_transactions (只读来源事实)
  -> ledger_v2_normalized_entries (标准化资金事实)
  -> ledger_v2_human_transactions (支付平台主交易)
  -> ledger_v2_transaction_overrides (人工五维解释)
```

- 支付宝和微信是主交易来源，标题优先级为商品说明、交易对方、来源分类、待解释。
- 招商银行流水只生成资金证据。仅同用户、同日、同金额、同方向时生成待确认候选；没有订单号或强商户证据绝不自动合并。
- 交易关闭和零元记录不计支出；退款、还款和内部流转均维持独立口径。
- 零元支付成功记录保留为独立证据，不在缺少来源状态依据时擅自解释为取消订单。
- 人工只能编辑标题、备注和五维解释；金额、状态、来源与原始证据永远只读。

## 本地存储与常用验证

- 原件存储键：`storage/ledger/users/{userID}/imports/...`，目录已被 Git 忽略。
- 本机数据验收（不输出账单内容）：

```bash
SHOREOS_RUN_LOCAL_LEDGER_INTEGRATION=1 \
SHOREOS_MYSQL_DEFAULTS_FILE=/absolute/path/to/shoreos_agent.cnf \
SHOREOS_MYSQL_SOCKET=/tmp/mysql.sock \
go test ./internal/repository/mysql -run TestLocalLedgerMaterializationAcceptance -count=1
```

- 常规后端测试：`go test ./...`
