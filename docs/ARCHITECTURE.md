# 团购后端架构文档

## 一、一个请求的生命周期

晚上 8 点，邻居张阿姨在微信里点开小程序，看到王团长发的「智利车厘子 JJJ 5斤装 128 元」，点了「我要一份」，填好自提点和手机号，点提交。

从这个动作开始，请求走过的完整路径是：

```
张阿姨的手指
  │
  ▼
POST /api/orders  {"group_id":1, "product_id":3, "buyer_name":"张阿姨", ...}
  │
  ▼
┌─ gin 路由匹配 ──────────────────────────────────────────┐
│  main.go 里注册了 auth.POST("/orders", handler.CreateOrder)  │
│  请求先经过 middleware.LeaderAuth 校验团长身份               │
└──────────────────────────────────────────────────────┘
  │
  ▼
┌─ middleware 层 ────────────────────────────────────────┐
│  LeaderAuth 从 Authorization 头里取 Bearer token       │
│  token 就是团长手机号，查 leaders 表确认存在             │
│  把 leader_id 写进 gin.Context，后续 handler 直接取      │
└──────────────────────────────────────────────────────┘
  │
  ▼
┌─ handler 层 ──────────────────────────────────────────┐
│  handler.CreateOrder (order.go)                        │
│  1. ShouldBindJSON 绑定请求参数                         │
│  2. 查 group 确认团期开着、没过截单时间                   │
│  3. 查 product 确认团品属于该团期                        │
│  4. 调 service.Stock.Deduct 扣库存（乐观锁）             │
│  5. 算总价、生成提货码、创建订单记录                      │
│  6. 返回 201 + 订单 JSON                               │
└──────────────────────────────────────────────────────┘
  │
  ▼
┌─ service 层 ──────────────────────────────────────────┐
│  service.Stock.Deduct (stock.go)                       │
│  1. 事务内 SELECT 查 product（拿 version）              │
│  2. 检查 stock - stock_sold >= qty                     │
│  3. UPDATE ... WHERE version = ? 原子扣减               │
│  4. RowsAffected == 1 才算成功，否则重试                 │
│  5. 超过 5 次重试返回售罄                               │
└──────────────────────────────────────────────────────┘
  │
  ▼
┌─ model + db 层 ───────────────────────────────────────┐
│  gorm 操作 SQLite，AutoMigrate 自动建表                 │
│  Product.AfterFind 钩子自动计算 StockRemaining          │
│  全局单例 db.DB 在 main.go 启动时初始化                 │
└──────────────────────────────────────────────────────┘
  │
  ▼
201 {"id":42, "pickup_code":"384721", "status":"pending", ...}
```

张阿姨看到「下单成功，提货码 384721」，整个流程结束。

---

## 二、目录职责

```
project153/
├── main.go                  入口：配置加载 → DB初始化 → 定时任务 → 路由注册 → 启动
├── config.yaml              运行时配置
├── internal/
│   ├── config/              配置加载，把 YAML 反序列化到结构体
│   ├── db/                  数据库连接 + AutoMigrate，全局 db.DB 单例
│   ├── model/               7 张表的数据模型 + GORM 钩子
│   ├── middleware/          HTTP 中间件：团长鉴权 + CORS
│   ├── handler/             4 个接口模块，负责参数绑定 + 响应组装
│   ├── service/             业务逻辑层，目前只有 StockService
│   └── scheduler/           cron 定时任务：截单 + 月度佣金
├── uploads/                 团品图片存放目录
├── export/                  CSV 导出存放目录
└── docs/                    文档
```

### 各层职责一句话总结

| 层 | 职责 | 不做什么 |
|---|---|---|
| **handler** | 参数绑定、校验、调用 service/直接查 DB、组装 HTTP 响应 | 不碰事务细节、不写业务逻辑 |
| **service** | 跨表事务编排、并发控制（乐观锁重试）、库存原子操作 | 不碰 HTTP、不管路由 |
| **model** | 数据结构定义 + GORM 钩子（如 AfterFind 算虚拟字段） | 不写查询逻辑 |
| **middleware** | 请求拦截：鉴权、CORS | 不处理业务 |
| **db** | 连接初始化 + AutoMigrate | 不写查询 |
| **config** | 读取 YAML → 全局结构体 `config.C` | 不做运行时逻辑 |
| **scheduler** | 注册 cron 回调，调用 handler 的公开函数 | 不写业务逻辑 |

---

## 三、核心链路详解

### 3.1 配置加载

[main.go](file:///d:/code/ai-prompt/solo-chrome-dev-F12/repos/repo153/project153/main.go#L16-L18) 启动时第一步：

```go
config.Load("config.yaml")
```

[config.Load](file:///d:/code/ai-prompt/solo-chrome-dev-F12/repos/repo153/project153/internal/config/config.go#L47-L53) 读 YAML 文件，反序列化到全局变量 `config.C`，之后所有模块通过 `config.C.Server.Port`、`config.C.Commission.Rate` 等读取配置。

### 3.2 团长鉴权流程

[LeaderAuth](file:///d:/code/ai-prompt/solo-chrome-dev-F12/repos/repo153/project153/internal/middleware/auth.go#L12-L31) 中间件挂在 `auth` 路由组上：

1. 从 `Authorization: Bearer <phone>` 取 token
2. 用 token（即手机号）查 `leaders` 表
3. 找到就把 `leader_id` 和 `leader` 对象写进 `gin.Context`
4. handler 里 `c.GetUint("leader_id")` 取当前团长 ID

公开接口（注册、登录、看进行中的团）走 `/api` 不经过此中间件；所有业务操作走 `/api` + LeaderAuth。

### 3.3 每晚截单链路

[scheduler.Start](file:///d:/code/ai-prompt/solo-chrome-dev-F12/repos/repo153/project153/internal/scheduler/scheduler.go#L13-L42) 注册 cron 表达式 `0 22 * * *`（每晚 22:00），触发时调用 [handler.RunCutoff](file:///d:/code/ai-prompt/solo-chrome-dev-F12/repos/repo153/project153/internal/handler/delivery.go#L22-L38)：

```
22:00 cron 触发
  │
  ▼
UPDATE groups SET status='closed' WHERE status='open' AND cutoff_at <= NOW()
  │
  ▼
遍历刚关闭的团期 → generateDeliveryList
  │
  ├─ 查该团下所有未取消订单
  ├─ 创建 DeliveryList（次日日期）
  ├─ 为每个订单创建 DeliveryItem（含库存快照）
  └─ 把订单状态推进为 arrived
```

团长也可以通过 `POST /api/deliveries/cutoff` 手动触发截单。

### 3.4 月度佣金批处理

[scheduler.Start](file:///d:/code/ai-prompt/solo-chrome-dev-F12/repos/repo153/project153/internal/scheduler/scheduler.go#L27-L38) 注册 cron `0 1 28 * *`（每月 28 号凌晨 1 点），遍历所有团长，为每人调用 [CalculateLeaderCommission](file:///d:/code/ai-prompt/solo-chrome-dev-F12/repos/repo153/project153/internal/handler/commission.go#L27-L70)：

```
计算上月（2026-05）佣金
  │
  ├─ 查该团长上月非取消订单的 COUNT 和 SUM(total_price)
  ├─ 佣金率取团长自己的 commission_rate，为 0 则回退到 config 里的 10%
  ├─ 佣金金额 = 总金额 × 佣金率
  └─ INSERT 或 UPDATE commissions 表
```

佣金状态流转：`pending` → `settled`（团长手动标记已结算）。

### 3.5 库存扣减链路（乐观锁）

下单时 [CreateOrder](file:///d:/code/ai-prompt/solo-chrome-dev-F12/repos/repo153/project153/internal/handler/order.go#L27-L105) 直接调用 [service.Stock.Deduct](file:///d:/code/ai-prompt/solo-chrome-dev-F12/repos/repo153/project153/internal/service/stock.go#L40-L67)：

```
Deduct(tx, productID, qty)
  │
  ├─ 事务内 SELECT product（拿到 version）
  ├─ stock <= 0 → 不限量，直接返回成功
  ├─ stock - stock_sold < qty → 返回 ErrStockUnavailable
  ├─ UPDATE products SET stock_sold=stock_sold+?, version=version+1
  │  WHERE id=? AND version=?
  │
  ├─ RowsAffected == 1 → 成功
  └─ RowsAffected == 0 → 被别人抢了，退避重试（最多 5 次）
```

取消订单时 [CancelOrder](file:///d:/code/ai-prompt/solo-chrome-dev-F12/repos/repo153/project153/internal/handler/order.go#L145-L183) 调用 [service.Stock.Restore](file:///d:/code/ai-prompt/solo-chrome-dev-F12/repos/repo153/project153/internal/service/stock.go#L69-L97)，同样带乐观锁重试，把 `stock_sold` 减回去。

Product 表的 `version` 字段就是乐观锁的版本号，每次 UPDATE 都 +1，保证高并发下不会超卖。

---

## 四、数据模型关系

```
Leader 1──────N Group 1──────N Product
  │                │
  │                └──────N Order ──────┐
  │                     │               │
  │                     │          (buyer)
  │                     │
  └──────N Commission   └──────N DeliveryItem ──┐
                                                   │
                            DeliveryList 1──────N ─┘
```

- 一个团长可以开多个团期（Group）
- 一个团期包含多个团品（Product）
- 一个团品可以被多个邻居下单（Order）
- 截单后，一个团期的订单聚合为一份配送清单（DeliveryList），清单里每条明细（DeliveryItem）对应一个订单
- 每月为每个团长生成一条佣金记录（Commission）

### 订单状态流转

```
pending → arrived → picked_up
  │
  └→ cancelled（可从 pending 直接取消，库存回滚）
```

### 库存字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `stock` | int | 总库存，0 表示不限量 |
| `stock_sold` | int | 已售出数量，Deduct +1 / Restore -1 |
| `version` | int | 乐观锁版本号，每次更新 +1 |
| `stock_remaining` | int | 虚拟字段（`gorm:"-"`），AfterFind 钩子自动计算 |

---

## 五、API 路由总览

### 公开接口（无需鉴权）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/leaders` | 注册团长 |
| GET | `/api/leaders/login?phone=` | 登录 |
| GET | `/api/groups/open` | 查看进行中的团 |

### 需要鉴权的接口（Bearer token = 手机号）

**团品模块**

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/groups` | 开团 |
| GET | `/api/groups` | 我的团期列表 |
| GET | `/api/groups/:id` | 团期详情（含团品） |
| POST | `/api/groups/:group_id/products` | 上团品 |
| GET | `/api/groups/:group_id/products` | 团品列表 |
| POST | `/api/products/:id/image` | 上传团品图片 |

**订单模块**

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/orders` | 下单 |
| GET | `/api/orders` | 订单列表（可按日期/团期/状态筛选） |
| GET | `/api/orders/:id` | 订单详情 |
| PUT | `/api/orders/:id/cancel` | 取消订单（库存回滚） |
| GET | `/api/orders/export` | 导出 CSV |

**配送清单模块**

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/deliveries/cutoff` | 手动截单 |
| GET | `/api/deliveries` | 配送清单列表 |
| GET | `/api/deliveries/:id` | 清单详情 |
| PUT | `/api/deliveries/:id/arrive` | 标记到货 |
| POST | `/api/deliveries/verify` | 核销提货码 |

**佣金模块**

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/commissions/calculate` | 触发佣金计算 |
| GET | `/api/commissions` | 佣金列表 |
| PUT | `/api/commissions/:id/settle` | 标记已结算 |

---

## 六、部署指南

### config.yaml 字段说明

```yaml
server:
  port: 8080              # 监听端口

database:
  driver: sqlite           # 目前只支持 sqlite
  dsn: "./groupbuy.db"     # 数据库文件路径

upload:
  dir: "./uploads"         # 图片上传目录
  max_size_mb: 5           # 单张图片最大 MB

export:
  dir: "./export"          # CSV 导出目录

scheduler:
  cutoff_hour: 22          # 每晚截单时间（24小时制）
  commission_day: 28       # 每月几号跑佣金批处理

commission:
  rate: 0.10               # 默认佣金率 10%，团长可单独覆盖
```

### 目录挂载建议

生产部署时建议把持久化数据挂载出来：

```bash
docker run -d \
  -p 8080:8080 \
  -v /data/groupbuy/groupbuy.db:/app/groupbuy.db \
  -v /data/groupbuy/uploads:/app/uploads \
  -v /data/groupbuy/export:/app/export \
  -v /data/groupbuy/config.yaml:/app/config.yaml \
  groupbuy:latest
```

- **groupbuy.db** — SQLite 数据库文件，必须持久化
- **uploads/** — 团品图片，丢了图片链接就 404
- **export/** — CSV 导出文件，可定期清理
- **config.yaml** — 按环境覆盖端口、佣金率等

### 数据库迁移

项目使用 GORM AutoMigrate，启动时自动执行：

```go
db.DB.AutoMigrate(
    &model.Leader{},
    &model.Group{},
    &model.Product{},
    &model.Order{},
    &model.DeliveryList{},
    &model.DeliveryItem{},
    &model.Commission{},
)
```

- **新增字段**：直接在 model 结构体加字段，重启服务后 AutoMigrate 自动加列
- **删字段**：AutoMigrate 不会删列，旧列保留不影响运行
- **改字段类型**：需要手动 `ALTER TABLE` 或重建数据库
- **首次部署**：启动服务自动建表，无需手动执行 SQL

### 本地运行

```bash
go run main.go
# 或
go build -o groupbuy.exe . && ./groupbuy.exe
```

### 跑测试

```bash
go test -v ./internal/service/...
```

测试使用临时 SQLite 文件（纯 Go 驱动，不需要 GCC），测试完自动清理。
