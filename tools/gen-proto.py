#!/usr/bin/env python3
# gen-proto.py <proto_dir> <go_out> <module>
import sys, subprocess
from pathlib import Path

proto_dir = Path(sys.argv[1]).resolve()
go_out    = Path(sys.argv[2]).resolve()
module    = sys.argv[3]

files = list(proto_dir.rglob("*.proto"))
if not files:
    sys.exit(f"no .proto files found in {proto_dir}")

cmd = [
    "protoc",
    f"--proto_path={proto_dir}",
    f"--go_out={go_out}", f"--go_opt=module={module}",
    f"--go-grpc_out={go_out}", f"--go-grpc_opt=module={module}",
    *[str(f) for f in files],
]
print(" ".join(cmd))
subprocess.run(cmd, check=True)
