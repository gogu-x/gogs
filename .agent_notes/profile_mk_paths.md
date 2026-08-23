# 项目路径映射说明 (来源: /data/330gs/gs/Profile.mk)

```
export CONF_OUTPUT_PATH=/data/330gs/gs/gdconf
export CONF_DB_TAG=sunshaoyi_h5
export CONF_PROJ_PATH=/data/330gs/gdconfig
export PROTO_PATH=/data/330gs/protocol
export PROTO_OUTPUT_PATH=/data/330gs/gs/pb
export CONF_I18N_PATH=/data/330gs/gdconfig/i18n
```

- proto 源文件目录: `/data/330gs/protocol` (例如 `/data/330gs/protocol/pb/cspb/def.proto`)
- proto 生成代码目录: `/data/330gs/gs/pb` (例如 `/data/330gs/gs/pb/cspb/def.pb.go`，DO NOT EDIT)
- 策划配置源文件目录: `/data/330gs/gdconfig`
- 配置生成代码目录: `/data/330gs/gs/gdconf`

修改错误码 (ErrCode)、消息结构等，应到 `/data/330gs/protocol` 下对应 `.proto` 文件修改，再重新生成 pb 代码，而不是直接改 `gs/pb` 下的生成文件。
修改策划配置表，应到 `/data/330gs/gdconfig` 下找源文件，而不是改 `gs/gdconf` 下的生成文件。
