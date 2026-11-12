# 深拷贝生成工具

递归扫描 Go 包，为标记 `// +deepcopy-gen=true` 的结构体生成深拷贝方法。包注释可使用 `// +deepcopy-gen=package`，单个结构体可通过 `// +deepcopy-gen=false` 排除。

在仓库根目录执行：

```sh
make deepcopy
make deepcopy GEN_DIR=game/play/internal/player
```

命令直接使用已编译文件，不执行编译。路径在 `config.mk` 的 `DEEPCOPY_GEN_BIN` 中配置，相对路径以仓库根目录为基准，也可填写绝对路径。移动工具后只需修改该配置；命令行也可通过同名变量覆盖。

运行 `make install-generators` 将生成器安装到 PATH 中的 `~/.local/bin`。源文件中直接声明：

```go
//go:generate deepcopy-gen
```

`go generate` 自动在源文件所在包执行，工具默认递归扫描当前目录及子包，不需要 `make -C` 或 `GEN_DIR`。例如 `go generate ./game/play/internal/player` 只处理玩家包及其子包，不再重复扫描整个 `game` 目录。要手工指定其他范围，仍可使用 `deepcopy-gen -dir 路径`。

工具在标记所在包生成 `deepcopy.go`，支持指针、映射、切片、数组及其嵌套组合。遇到不支持的类型或未标记的结构体会报错。

## 公共类型引用

`-reference-dir` 指定仅用于识别类型标记的依赖源码目录，依赖类型不会加入生成输出列表。Makefile 默认不加载外部目录；需要时通过 `DEEPCOPY_REFERENCE_DIR` 显式指定依赖目录。依赖中的 DeepCopy 由依赖库自身负责生成。
