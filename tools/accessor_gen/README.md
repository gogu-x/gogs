# 玩家结构访问方法生成

工具为简单结构体生成空指针安全的读取方法和字段设置方法，适用于不含导入类型和嵌入字段的结构体。

在 `gogs` 根目录运行：

```powershell
make accessor GEN_DIR=game/play/internal/player/souljade STRUCTS="Mgr Item Affix"
make accessor GEN_DIR=game/play/internal/player/talent STRUCTS=Mgr
make accessor GEN_DIR=battle/battle/internal/configcompiler STRUCTS=RoleAttributes
make deepcopy GEN_DIR=game/play/internal/player
```

命令直接执行已编译文件，不编译工具。访问方法、深拷贝和活动路由工具路径统一在 `config.mk` 的 `ACCESSOR_GEN_BIN`、`DEEPCOPY_GEN_BIN`、`ROUTER_GEN_BIN` 中配置。相对路径以仓库根目录为基准，也可填写绝对路径，或在命令行覆盖同名变量。

结构体上的 `go:generate` 标签通过 `make -C` 定位仓库根目录，再传入 `GEN_DIR` 和 `STRUCTS`，工具移动后只需更新配置。例如天赋包使用：

```go
//go:generate make -C ../../../../.. accessor GEN_DIR=game/play/internal/player/talent STRUCTS=Mgr
```

活动路由生成使用 `make router GEN_DIR=game/actv/internal/template/online`，相关标签也共用配置。工具不替换协议或配置生成器，不对整个旧模块批量补生成。

`*_accessors.go` 与 `deepcopy.go` 均为生成物，不应手改。标准库 AST 本身没有 Getter，生成器按 Go AST API 读取其字段；已有 Player 结构继续沿用该文件原有的手写 Getter 模式，避免覆盖其既有行为。
