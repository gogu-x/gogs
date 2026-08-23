# Kibana 日志采集平台（h5ftp）

- Kibana 地址：`http://106.53.123.224:5601`（Kibana 8.x）
- Basic Auth：用户名 `suhongfan`；密码 `123456`
- data view：`684fa562-cfd9-40d8-971f-d421c1a044f9`
- 生产真实索引：`game-h5prod-*`

## 查询 API

无需登录网页，使用 Kibana Console Proxy 代理 ES 搜索：

```bash
curl -u "suhongfan:123456" -X POST \
  "http://106.53.123.224:5601/api/console/proxy?path=/game-h5prod-*/_search&method=POST" \
  -H "kbn-xsrf: true" -H "Content-Type: application/json" \
  -d '{"size":20,"sort":[{"@timestamp":"desc"}],"query":{"bool":{"must":[{"query_string":{"query":"message:\"...\""}},{"range":{"@timestamp":{"gte":"...","lte":"..."}}}]}},"_source":["@timestamp","serverid","src_location","message"]}'
```

## 要点

- 通过 `/api/console/proxy` 到 ES `_search`；`path` 使用真实索引 `game-h5prod-*`，不是 data view ID。
- `@timestamp` 必须使用 UTC，例如 `2026-08-17T16:00:00.000Z`。
- 日志内容用 ES `query_string` 语法，例如 `message:\"关键词\"`。
- 公开端点仅 `/api/status`、`/login`；数据 API 必须携带 Basic Auth。
- 可请求 `/api/data_views` 列出所有 data view。
