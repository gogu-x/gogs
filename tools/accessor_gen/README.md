# 玩家结构访问方法生成

工具为简单结构体生成空指针安全的读取方法和字段设置方法，适用于不含导入类型和嵌入字段的结构体。

在 `gogs` 根目录运行：

```powershell
make accessor GEN_DIR=game/play/internal/player/souljade STRUCTS="Mgr Item Affix"
make accessor GEN_DIR=game/play/internal/player/talent STRUCTS=Mgr
make accessor GEN_DIR=battle/battle/internal/battleconfig STRUCTS=RoleAttributes
make deepcopy GEN_DIR=game/play/internal/player
```

命令直接执行已编译文件，不编译工具。访问方法、深拷贝和活动路由工具路径统一在 `config.mk` 的 `ACCESSOR_GEN_BIN`、`DEEPCOPY_GEN_BIN`、`ROUTER_GEN_BIN` 中配置。相对路径以仓库根目录为基准，也可填写绝对路径，或在命令行覆盖同名变量。

先在仓库根目录运行 `make install-generators`，安装 `accessor-gen`、`deepcopy-gen` 、`actv-router-gen` 和 `conf-gen` 到 `~/.local/bin`（该目录需在 PATH 中）。链接指向当前仓库的编译产物，更新工具源码后重新运行安装命令即可。

`go generate` 在源文件所在包目录执行，因此不再重复填写目录：

```go
//go:generate accessor-gen
type Mgr struct { /* 字段 */ }
//go:generate deepcopy-gen
//go:generate actv-router-gen
```

accessor 指令放在目标结构体声明上方，利用 `go generate` 提供的 `GOFILE`、`GOLINE` 自动定位，无需填写结构体名。直接运行无参数 `accessor-gen` 会扫描当前包所有带此指令的结构体，不处理未标记结构体。仍支持 `accessor-gen Mgr Item` 显式指定名称。其余目录默认为当前包。deepcopy 递归扫描当前目录及子包。原来的 `make accessor/deepcopy/router GEN_DIR=...` 仍可用于显式指定扫描范围。

`*_accessors.go` 与 `deepcopy.go` 均为生成物，不应手改。标准库 AST 本身没有 Getter，生成器按 Go AST API 读取其字段；已有 Player 结构继续沿用该文件原有的手写 Getter 模式，避免覆盖其既有行为。

配置生成标签使用 `//go:generate conf-gen`，从当前包自动向上查找 Go 模块根目录，再调用现有 `make conf-code`，保留 `config.mk` 配置，不执行导库。
