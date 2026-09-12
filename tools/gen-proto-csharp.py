#!/usr/bin/env python3
# gen-proto-csharp.py <proto_dir> <csharp_out>
#
# Unity 生成物放在同一目录。仓库既有约定先把 cspb/battle/struct.proto
# 扁平化为 cspb_battle_struct.proto，因此 protoc 会生成唯一的
# CspbBattleStruct.cs / CspbBattleStructReflection，避免各模块的
# StructReflection、ReqReflection、AckReflection 在全局命名空间冲突。
import re
import subprocess
import sys
import tempfile
from pathlib import Path

proto_dir = Path(sys.argv[1]).resolve()
csharp_out = Path(sys.argv[2]).resolve()
csharp_out.mkdir(parents=True, exist_ok=True)

client_root = proto_dir / "cspb"
files = sorted(client_root.rglob("*.proto"))
if not files:
    sys.exit(f"no client .proto files found in {client_root}")


def flat_name(source: Path) -> str:
    return "_".join(source.relative_to(proto_dir).parts)


names = {source.relative_to(proto_dir).as_posix(): flat_name(source) for source in files}
with tempfile.TemporaryDirectory() as directory:
    temporary = Path(directory)
    staged = []
    for source in files:
        text = source.read_text(encoding="utf-8")
        text = re.sub(
            r'import\s+"([^"]+)"\s*;',
            lambda match: f'import "{names.get(match.group(1), match.group(1))}";',
            text,
        )
        destination = temporary / flat_name(source)
        destination.write_text(text, encoding="utf-8")
        staged.append(destination)

    cmd = [
        "protoc",
        f"--proto_path={temporary}",
        f"--csharp_out={csharp_out}",
        *(str(path) for path in staged),
    ]
    print(" ".join(cmd))
    subprocess.run(cmd, check=True)
