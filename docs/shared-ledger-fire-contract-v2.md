# ShoreOS 滚动支出与 FIRE 目标联动合同 v2

## 状态与边界

- 状态：已确认，Ledger V3 与 FIRE V2 共同遵守。
- 账本负责真实支出、预算、准备金和滚动支出基线。
- FIRE 负责手工收入/资产/目标参数、测算运行和支出影响分析。
- 两个模块共用身份会话与可信 `userID`，但不跨模块直接写表。
- 不接工资单，不增加支付宝 CSV、微信 XLSX、招商银行 PDF 之外的来源。

```text
账本真实支出
+ 固定支出计划
+ 浮动预算
+ 大额准备金
        -> 滚动支出基线（7 / 30 / 90 / 365 天）
        -> FIRE 目标线 / 实际线
        -> 账本展示节余、亏空和主要影响交易
```

默认读取时实时计算，默认窗口为 30 天。正式 FIRE 测算保存完整输入快照，不设置每日定时任务。

## 不变量

1. 请求体和查询参数不得传数据归属用户；服务端只从 Bearer 会话取得 `userID`。
2. 原始账单、金额、状态、来源和证据只读；预算与人工解释不得覆盖来源事实。
3. 交易关闭、零元记录、退款、内部流转和待完成均不进入实际支出。
4. 招商银行流水只作为资金证据或人工关联候选，不自动成为消费主交易。
5. 信息不足必须显示 `unknown` 或待解释，不调用模型强行分类。
6. FIRE 不直接查询 `ledger_v2_*` 明细表，只调用账本能力取得基线或影响输入。

## 预算模型

现有五维保持不变。新增独立的 `budgetTreatment`：

- `fixed`：用户确认的固定承诺，年度化金额来自固定支出计划。
- `flexible`：日常浮动支出，受用户确认的月度额度约束。
- `funded_irregular`：可预见的大额支出，必须关联准备金用途池。
- `exceptional`：无法提前规划的一次性支出，保留真实现金影响但不年化。

未人工设置的消费默认按 `flexible` 处理。人工单笔覆盖优先于后续规则；预算处理方式不改变五维语义。

浮动额度由用户确认，近 30/90 天实际水平只作为建议，系统不得自动上调。累计余额为：

```text
累计浮动余额 = 启用月份数 × 月度浮动预算 - 启用后的浮动实际支出
```

正数为节余，负数为亏空。系统只提醒，不自动扣减后续月份额度。

## 准备金

每个用途池包含名称、月度预留额、期初余额、开始日期、状态和可选的停止计提日期。

```text
当前余额
= 期初余额
+ 启用月份数 × 月度预留额
- 已关联的人类交易实际支出
```

- 余额可跨月、跨年，也允许为负。
- 不自动在用途池之间调拨。
- 已暂停或关闭的池第一版不能重新启用；需要继续时新建用途池。
- 月度预留不是实际账单消费，但进入长期年度支出。
- 正的准备金余额从 FIRE 可投资资产中扣除；负余额不能反向增加可投资资产。
- 准备金支持的消费不重复造成长期拖慢；未准备的一次性支出提供反事实影响。

## 滚动基线接口

```text
GET /api/v1/ledger/spending-baseline?windowDays=30&asOf=YYYY-MM-DD
```

`windowDays` 只接受 `7 / 30 / 90 / 365`。

时间工作台可改用精确范围：

```text
GET /api/v1/ledger/spending-baseline?from=YYYY-MM-DD&to=YYYY-MM-DD
```

`from/to` 必须同时提供，范围为 7—366 天，且不能与 `windowDays/asOf` 混用。旧窗口调用保持兼容。

```ts
type RollingSpendingBaseline = {
  asOf: string
  windowDays: 7 | 30 | 90 | 365
  coveredFrom: string
  coveredTo: string
  currency: 'CNY'
  hasImportedData: boolean
  actualExpenseCents: number
  fixedAnnualCents: number
  flexibleActualCents: number
  flexibleBudgetMonthlyCents: number
  flexibleBalanceCents: number
  sinkingFundAnnualContributionCents: number
  sinkingFundBalanceCents: number
  exceptionalActualCents: number
  targetAnnualExpenseCents: number
  actualPaceAnnualExpenseCents: number
  pendingAmountCents: number
  refundAmountCents: number
  internalTransferAmountCents: number
  needsReviewExpenseCents: number
  dataCoverage: 'complete' | 'partial' | 'insufficient'
}
```

```text
目标年度支出
= 已确认固定月支出 × 12
+ 浮动月度预算 × 12
+ 活跃准备金月度预留 × 12

实际节奏年度支出
= 已确认固定月支出 × 12
+ 窗口内浮动支出 ÷ 覆盖天数 × 365
+ 活跃准备金月度预留 × 12
```

`exceptional`、退款、内部流转、待完成、关闭交易和零元记录均不进入年度节奏。覆盖不足时仍返回结果，但必须标记数据可信度。

## FIRE 合同

收入、资产、收益率、通胀、最低生活成本和目标参数继续由用户手工维护。最低长期生活成本决定 FIRE 目标金额；账本基线只改变当前年度支出和可投入速度，不改变同一情景的 FIRE 目标。

```text
目标线：targetAnnualExpenseCents
实际线：actualPaceAnnualExpenseCents
可投资资产：手工资产 - 正的准备金余额 - 其他既有不可投资项
```

正式测算：

```text
POST /api/v1/fire/projection-runs
```

```ts
type FireProjectionInput = {
  scenarioId: string
  spendingMode: 'manual' | 'ledger'
  windowDays?: 7 | 30 | 90 | 365
  asOf?: string
}
```

`ledger` 模式必须把情景、滚动基线、模型输入、目标线和实际线完整写入 `input_snapshot`。后续账本变化不得改写历史运行。

账本影响读取：

```text
GET /api/v1/fire/scenarios/{scenarioId}/spending-impact?windowDays=30&asOf=YYYY-MM-DD
```

时间工作台使用与滚动基线完全相同的精确范围：

```text
GET /api/v1/fire/scenarios/{scenarioId}/spending-impact?from=YYYY-MM-DD&to=YYYY-MM-DD
```

单日或不足 7 天的事实范围由前端扩展为“截至所选结束日的最近 7 天”后调用；正式测算快照接口不变。

返回目标线/实际线日期差、浮动节余或亏空、主要分类与交易、一次性支出反事实结果和准备金状态。普通单笔交易只显示预算占用；仅 `exceptional` 显示“假设这笔钱仍可投资”的日期差。

## 实施与验收

时间范围事实聚合：

```text
GET /api/v1/ledger/period-analysis?from=YYYY-MM-DD&to=YYYY-MM-DD&bucket=day|month
```

- 期间范围为 1—366 天；`day` 用于月历与日/区间，`month` 用于年度趋势。
- 实际支出、收入、退款、内部流转、待完成、已关闭、零元和待解释分别汇总。
- 五个结构维度只聚合实际支出；所有查询都以认证用户为隔离条件。

- 预算计划：`GET/PUT /api/v1/ledger/budget-plan`。
- 准备金：`POST /api/v1/ledger/sinking-funds` 与 `PATCH /api/v1/ledger/sinking-funds/{id}`。
- 人类交易解释接口扩展 `budgetTreatment` 与 `sinkingFundId`。
- 后端负责所有金额、余额、年度化和 FIRE 日期计算；Taro 只展示返回结果。
- 用户 A 不能读取或修改用户 B 的预算、准备金、基线、情景或测算。
- 7/30/90/365 天窗口不得让固定支出消失，也不得年化一次性异常支出。
- 准备金累计、负余额、暂停计提、历史测算不可变均必须有测试。
