# 深拷贝生成工具

递归扫描 Go 包，为标记 `// +deepcopy-gen=true` 的结构体生成深拷贝方法。包注释可使用 `// +deepcopy-gen=package`，单个结构体可通过 `// +deepcopy-gen=false` 排除。

在仓库根目录执行：

```sh
make deepcopy
make deepcopy GEN_DIR=game/play/internal/player
```

命令直接使用已编译文件，不执行编译。路径在 `config.mk` 的 `DEEPCOPY_GEN_BIN` 中配置，相对路径以仓库根目录为基准，也可填写绝对路径。移动工具后只需修改该配置；命令行也可通过同名变量覆盖。

结构体上的生成指令通过 Makefile 调用工具，例如玩家包使用：

```go
//go:generate make -C ../../../.. deepcopy GEN_DIR=game
```

其中 `-C` 从当前 Go 包指向仓库根目录，`GEN_DIR` 相对于仓库根目录。执行 `go generate ./game/play/internal/player` 即可调用对应指令。

工具在标记所在包生成 `deepcopy.go`，支持指针、映射、切片、数组及其嵌套组合。遇到不支持的类型或未标记的结构体会报错。
