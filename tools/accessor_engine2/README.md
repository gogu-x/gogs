# 玩家结构访问方法生成

仓库原约定使用 `accessor_engine2`，本机未安装相应工具。本目录提供范围受限、可复现的 Go AST 实现，为本次新增的简单玩家结构生成空指针安全的 Getter 和 Setter。

在 `gogs` 根目录运行：

```powershell
go run ./tools/accessor_engine2 -dir game/play/internal/player/souljade Mgr Item Affix
go run ./tools/accessor_engine2 -dir game/play/internal/player/talent Mgr
go run ./tools/accessor_engine2 -dir battle/battle/internal/configcompiler RoleAttributes
go run ./tools/deepcopy-gen -dir ./game/play/internal/player
```

也可以先 `go install ./tools/accessor_engine2`，之后直接执行结构体上的 `go:generate` 指令。工具仅面向不含导入类型和嵌入字段的简单结构，不替换 protobuf 或配置生成器，不对整个旧模块批量补生成。

`*_accessors.go` 与 `deepcopy.go` 均为生成物，不应手改。标准库 AST 本身没有 Getter，生成器按 Go AST API 读取其字段；已有 Player 结构继续沿用该文件原有的手写 Getter 模式，避免覆盖其既有行为。
