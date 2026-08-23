# MongoDB (mongosh) 常用语句整理

> 适用环境：`mongosh`（MongoDB Shell），语法为 JavaScript。
> 说明：文中 `db` 代表当前连接的数据库对象，`col`/集合名需替换为实际集合。

---

## 1. 连接与基础操作

```js
// 连接（在系统终端执行，而非 mongosh 内部）
mongosh "mongodb://用户名:密码@host:port/db_name?authSource=admin"

// 查看当前数据库
db

// 切换/创建数据库（插入数据后才会真正创建）
use mydb

// 查看所有数据库
show dbs
show databases

// 查看当前库下所有集合
show collections

// 查看帮助
db.help()
db.mycol.help()
```

---

## 2. 增（Create / Insert）

```js
// 插入单条文档
db.users.insertOne({ name: "Tom", age: 18, tags: ["a", "b"] })

// 插入多条文档
db.users.insertMany([
  { name: "Tom", age: 18 },
  { name: "Jerry", age: 20 }
])

// 旧版通用写法（单条/多条均可，不推荐新代码使用）
db.users.insert({ name: "Tom" })
```

---

## 3. 查（Read / Find）

### 3.1 基本查询

```js
// 查询所有文档
db.users.find()

// 格式化输出（更易读）
db.users.find().pretty()

// 查询单条（返回第一条匹配文档）
db.users.findOne({ name: "Tom" })

// 条件查询
db.users.find({ age: 18 })

// 指定返回字段（1=返回，0=不返回，_id 默认返回）
db.users.find({ age: 18 }, { name: 1, age: 1, _id: 0 })
```

### 3.2 比较运算符

```js
db.users.find({ age: { $gt: 18 } })     // 大于
db.users.find({ age: { $gte: 18 } })    // 大于等于
db.users.find({ age: { $lt: 18 } })     // 小于
db.users.find({ age: { $lte: 18 } })    // 小于等于
db.users.find({ age: { $ne: 18 } })     // 不等于
db.users.find({ age: { $in: [18, 20, 22] } })    // 在列表中
db.users.find({ age: { $nin: [18, 20] } })       // 不在列表中
```

### 3.3 逻辑运算符

```js
// AND（默认多条件即 AND）
db.users.find({ age: 18, name: "Tom" })

// 显式 $and
db.users.find({ $and: [{ age: 18 }, { name: "Tom" }] })

// OR
db.users.find({ $or: [{ age: 18 }, { name: "Tom" }] })

// NOT
db.users.find({ age: { $not: { $eq: 18 } } })

// NOR（都不满足）
db.users.find({ $nor: [{ age: 18 }, { name: "Tom" }] })
```

### 3.4 元素/数组/正则

```js
// 字段是否存在
db.users.find({ email: { $exists: true } })

// 字段类型判断
db.users.find({ age: { $type: "int" } })

// 数组包含某元素
db.users.find({ tags: "a" })

// 数组同时包含多个元素
db.users.find({ tags: { $all: ["a", "b"] } })

// 数组长度
db.users.find({ tags: { $size: 2 } })

// 数组中至少一个元素满足条件（对象数组）
db.orders.find({ items: { $elemMatch: { qty: { $gt: 5 }, name: "apple" } } })

// 正则匹配（模糊查询，类似 SQL LIKE）
db.users.find({ name: { $regex: "^To", $options: "i" } })
db.users.find({ name: /^To/i })
```

### 3.5 排序、分页、计数

```js
// 排序：1 升序，-1 降序
db.users.find().sort({ age: -1 })

// 跳过 N 条
db.users.find().skip(10)

// 限制返回条数
db.users.find().limit(5)

// 分页（跳过10条，取5条）
db.users.find().skip(10).limit(5)

// 计数
db.users.countDocuments({ age: 18 })
db.users.estimatedDocumentCount()   // 估算总数（不带条件，速度快）

// 去重（返回字段的不同取值）
db.users.distinct("age")
```

### 3.6 嵌套字段查询

```js
// 点号访问嵌套对象字段
db.users.find({ "address.city": "Beijing" })
```

---

## 4. 改（Update）

```js
// 更新单条（只更新第一条匹配的文档）
db.users.updateOne(
  { name: "Tom" },
  { $set: { age: 19 } }
)

// 更新多条
db.users.updateMany(
  { age: { $lt: 18 } },
  { $set: { status: "minor" } }
)

// 替换整个文档（除 _id 外全部替换）
db.users.replaceOne(
  { name: "Tom" },
  { name: "Tom", age: 19, status: "active" }
)

// upsert：不存在则插入，存在则更新
db.users.updateOne(
  { name: "Tom" },
  { $set: { age: 19 } },
  { upsert: true }
)
```

### 4.1 常用更新操作符

```js
db.users.updateOne({ name: "Tom" }, { $set: { age: 20 } })       // 设置字段值
db.users.updateOne({ name: "Tom" }, { $unset: { age: "" } })     // 删除字段
db.users.updateOne({ name: "Tom" }, { $inc: { age: 1 } })        // 自增(可为负数)
db.users.updateOne({ name: "Tom" }, { $mul: { age: 2 } })        // 自乘
db.users.updateOne({ name: "Tom" }, { $rename: { age: "years" } }) // 字段重命名
db.users.updateOne({ name: "Tom" }, { $min: { age: 10 } })       // 若新值更小才更新
db.users.updateOne({ name: "Tom" }, { $max: { age: 30 } })       // 若新值更大才更新
db.users.updateOne({ name: "Tom" }, { $currentDate: { lastModified: true } }) // 设为当前时间
```

### 4.2 数组更新操作符

```js
db.users.updateOne({ name: "Tom" }, { $push: { tags: "c" } })          // 追加单个元素
db.users.updateOne({ name: "Tom" }, { $push: { tags: { $each: ["c", "d"] } } }) // 追加多个
db.users.updateOne({ name: "Tom" }, { $addToSet: { tags: "c" } })      // 不重复才追加
db.users.updateOne({ name: "Tom" }, { $pop: { tags: 1 } })             // 移除最后一个(1)/第一个(-1)
db.users.updateOne({ name: "Tom" }, { $pull: { tags: "c" } })          // 移除指定元素
db.users.updateOne({ name: "Tom" }, { $pullAll: { tags: ["c", "d"] } })// 移除多个指定元素

// 更新数组中匹配条件的元素（$[] 全部元素 / $ 第一个匹配元素）
db.orders.updateOne(
  { "items.name": "apple" },
  { $set: { "items.$.qty": 100 } }
)
```

---

## 5. 删（Delete）

```js
// 删除单条
db.users.deleteOne({ name: "Tom" })

// 删除多条
db.users.deleteMany({ age: { $lt: 18 } })

// 删除集合内所有文档（保留集合结构）
db.users.deleteMany({})

// 删除整个集合（包括索引，慎用）
db.users.drop()

// 删除整个数据库（慎用！）
db.dropDatabase()
```

---

## 6. 聚合查询（Aggregation）

```js
// 基本聚合管道：先过滤，再分组统计，再排序
db.orders.aggregate([
  { $match: { status: "paid" } },                       // 过滤，等价于 find 条件
  { $group: { _id: "$userId", total: { $sum: "$amount" } } }, // 分组求和
  { $sort: { total: -1 } },                              // 按 total 降序
  { $limit: 10 }                                         // 取前10
])

// 常用 $group 累加器
db.orders.aggregate([
  { $group: {
      _id: "$userId",
      count: { $sum: 1 },        // 计数
      total: { $sum: "$amount" },// 求和
      avg:   { $avg: "$amount" },// 平均
      max:   { $max: "$amount" },// 最大
      min:   { $min: "$amount" },// 最小
      items: { $push: "$item" }  // 收集为数组
  } }
])

// $project：筛选/重命名/计算字段
db.orders.aggregate([
  { $project: { _id: 0, userId: 1, amount: 1, doubled: { $multiply: ["$amount", 2] } } }
])

// $lookup：类似 SQL 的 left join
db.orders.aggregate([
  { $lookup: {
      from: "users",
      localField: "userId",
      foreignField: "_id",
      as: "userInfo"
  } }
])

// $unwind：展开数组字段，每个元素变成一条文档
db.orders.aggregate([
  { $unwind: "$items" }
])

// 统计集合文档总数（等价于 countDocuments）
db.orders.aggregate([{ $count: "total" }])
```

---

## 7. 索引（Index）

```js
// 创建普通索引（1升序/-1降序）
db.users.createIndex({ age: 1 })

// 创建唯一索引
db.users.createIndex({ email: 1 }, { unique: true })

// 创建复合索引
db.users.createIndex({ age: 1, name: -1 })

// 创建 TTL 索引（文档过期自动删除，expireAfterSeconds 单位秒）
db.sessions.createIndex({ createdAt: 1 }, { expireAfterSeconds: 3600 })

// 创建文本索引（支持 $text 全文检索）
db.articles.createIndex({ content: "text" })

// 查看集合所有索引
db.users.getIndexes()

// 删除指定索引
db.users.dropIndex("age_1")

// 删除除 _id 之外的所有索引
db.users.dropIndexes()

// 查看查询执行计划（分析索引是否命中）
db.users.find({ age: 18 }).explain("executionStats")
```

---

## 8. 集合与数据库管理

```js
// 手动创建集合（可指定选项，如固定集合）
db.createCollection("logs", { capped: true, size: 1048576 })

// 重命名集合
db.users.renameCollection("members")

// 查看集合统计信息（大小、文档数等）
db.users.stats()

// 查看数据库统计信息
db.stats()

// 查看当前数据库大小
db.dataSize()
```

---

## 9. 用户与权限（需在 admin 库或对应库执行）

```js
// 创建用户
db.createUser({
  user: "appuser",
  pwd: "your_password",   // 生产环境建议用 passwordPrompt() 交互输入，避免密码留在脚本/历史记录中
  roles: [{ role: "readWrite", db: "mydb" }]
})

// 查看当前库所有用户
db.getUsers()

// 删除用户
db.dropUser("appuser")

// 查看当前登录用户权限
db.runCommand({ connectionStatus: 1 })
```

---

## 10. 事务（多文档事务，需副本集/分片集群）

```js
const session = db.getMongo().startSession();
const sessionDb = session.getDatabase("mydb");

session.startTransaction();
try {
  sessionDb.accounts.updateOne({ _id: "A" }, { $inc: { balance: -100 } });
  sessionDb.accounts.updateOne({ _id: "B" }, { $inc: { balance: 100 } });
  session.commitTransaction();
} catch (e) {
  session.abortTransaction();
  throw e;
} finally {
  session.endSession();
}
```

---

## 11. 备份与恢复（命令行工具，非 mongosh 内命令）

```bash
# 备份整个数据库
mongodump --uri="mongodb://host:port/db_name" --out=/backup/path

# 恢复数据库
mongorestore --uri="mongodb://host:port" /backup/path

# 导出集合为 JSON
mongoexport --uri="mongodb://host:port/db_name" --collection=users --out=users.json

# 导入 JSON 到集合
mongoimport --uri="mongodb://host:port/db_name" --collection=users --file=users.json
```

---

## 12. 常用运维/排查命令

```js
// 查看当前正在执行的操作
db.currentOp()

// 终止指定操作（需要 opid，从 currentOp 结果中获取）
db.killOp(opid)

// 查看慢查询日志配置（100ms 以上记录）
db.setProfilingLevel(1, { slowms: 100 })

// 查看 profile 记录
db.system.profile.find().sort({ ts: -1 }).limit(10)

// 查看服务器状态
db.serverStatus()

// 查看副本集状态
rs.status()

// 查看分片集群状态
sh.status()
```

---

## 13. 脚本执行方式（终端命令）

```bash
# 直接执行 js 文件
mongosh "<连接串>/<db_name>" script.js

# 指定 --file 参数
mongosh --file script.js "<连接串>/<db_name>"

# 免交互执行单行代码
mongosh "<连接串>/<db_name>" --eval "db.users.countDocuments({})"

# 在 mongosh 交互环境内加载脚本
load("script.js")
```

---

## 注意事项

- `updateMany` / `deleteMany` / `dropDatabase` 等批量/破坏性操作影响范围大，生产环境务必先用 `find` 确认匹配范围，必要时先做数据备份。
- 涉及金额、库存等关键字段的批量更新，建议先在测试环境验证，或加 `dryRun` 式的打印确认逻辑再落库。
- 密码等敏感信息不要写在脚本明文里或提交到版本库，优先使用环境变量、`passwordPrompt()` 或密钥管理服务。
