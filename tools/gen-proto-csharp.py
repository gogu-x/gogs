#!/usr/bin/env python3
# gen-proto-csharp.py <proto_dir> <csharp_out>
#
# 从同一份 .proto 源文件（与 gen-proto.py 生成 Go 代码共用）生成 C# 代码，
# 供 Unity 前端项目使用，保证前后端协议单一来源、不漂移。
#
# 用法示例（在 gogs 仓库根目录下执行）：
#   python tools/gen-proto-csharp.py protocol D:/untiyClinet/XiuXianGame/Assets/Scripts/Proto
import sys, subprocess
from pathlib import Path

proto_dir  = Path(sys.argv[1]).resolve()
csharp_out = Path(sys.argv[2]).resolve()

csharp_out.mkdir(parents=True, exist_ok=True)

files = list(proto_dir.rglob("*.proto"))
if not files:
    sys.exit(f"no .proto files found in {proto_dir}")

cmd = [
    "protoc",
    f"--proto_path={proto_dir}",
    f"--csharp_out={csharp_out}",
    *[str(f) for f in files],
]
print(" ".join(cmd))
subprocess.run(cmd, check=True)
