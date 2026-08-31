#!/usr/bin/env python3
# gen-proto.py <proto_dir> <go_out> <module>
#
# 在调用 protoc 之前，自动向每个以 Req 结尾的 message 内联公共头字段：
#   uint64 uID      = 1;
#   uint32 gateID   = 2;
#   uint32 serverID = 3;
# 业务字段号自动顺延（从 4 开始）。源 .proto 文件保持干净，注入发生在生成期。
import re, subprocess, sys, tempfile
from pathlib import Path

proto_dir = Path(sys.argv[1]).resolve()
go_out    = Path(sys.argv[2]).resolve()
module    = sys.argv[3]

# 注入到每个 *Req 的公共头字段（与 protocol/common/struct.proto 的 message comm 对齐）
REQ_HEADER_FIELDS = [
    "  uint64   uID       = 1;",
    "  uint32   serverID  = 2;",
    "  string   sessionID = 3;",
]
REQ_HEADER_END = 4  # 业务字段从此号开始

# 不注入公共头的模块目录：
#   - gateway：传输层消息，元数据已由 Frame 携带；
#   - platform：服务间 gRPC RPC（如 CreateOrderReq 已自带 uid 字段）。
EXCLUDE_MODULES = {"gateway", "platform"}


def _transform_req_block(body, name):
    """把 message XxxReq { ... } 整体改写：内联公共头字段并重排业务字段号。"""
    m = re.match(r"^(\s*message\s+\w+\s*\{)([\s\S]*?)(\}\s*)$", body)
    if not m:
        return body
    head, inner, tail = m.groups()
    if re.search(r"\buID\s*=", inner):
        return body  # 已注入过，跳过
    lines = inner.split("\n")
    new_lines = []
    field_no = REQ_HEADER_END
    for ln in lines:
        s = ln.strip()
        if not s or s.startswith("//"):
            new_lines.append(ln)
            continue
        fm = re.match(r"^(\s*)(.*?)=(\s*)(\d+)(\s*);(.*)$", ln)
        if fm:
            ind, pre, sp1, _num, sp2, rest = fm.groups()
            new_lines.append(f"{ind}{pre}={sp1}{field_no}{sp2};{rest}")
            field_no += 1
        else:
            new_lines.append(ln)
    # 在第一个有效字段前插入公共头
    out = []
    inserted = False
    for ln in new_lines:
        if not inserted and ln.strip() and not ln.strip().startswith("//"):
            out.extend(REQ_HEADER_FIELDS)
            inserted = True
        out.append(ln)
    if not inserted:
        out.extend(REQ_HEADER_FIELDS)
    return head + "\n" + "\n".join(out) + "\n" + tail


def inject_req_header(text):
    """扫描顶层 message 块，对 *Req 消息内联公共头字段。"""
    blocks = []
    i, n = 0, len(text)
    pattern = re.compile(r"message\s+(\w+)\s*\{")
    while True:
        m = pattern.search(text, i)
        if not m:
            break
        name = m.group(1)
        open_idx = m.end() - 1
        depth, j = 1, open_idx + 1
        while j < n and depth > 0:
            if text[j] == "{":
                depth += 1
            elif text[j] == "}":
                depth -= 1
            j += 1
        close_idx = j - 1
        blocks.append((m.start(), close_idx, name))
        i = close_idx + 1

    for start, close, name in reversed(blocks):
        if name.endswith("Req"):
            body = text[start : close + 1]
            text = text[:start] + _transform_req_block(body, name) + text[close + 1 :]
    return text


files = list(proto_dir.rglob("*.proto"))
if not files:
    sys.exit(f"no .proto files found in {proto_dir}")

with tempfile.TemporaryDirectory() as tmp:
    tmp = Path(tmp)
    protoc_files = []
    for f in files:
        rel = f.relative_to(proto_dir)
        dest = tmp / rel
        dest.parent.mkdir(parents=True, exist_ok=True)
        text = f.read_text(encoding="utf-8")
        if rel.parts and rel.parts[0] not in EXCLUDE_MODULES:
            text = inject_req_header(text)
        dest.write_text(text, encoding="utf-8")
        protoc_files.append(str(dest))

    cmd = [
        "protoc",
        f"--proto_path={tmp}",
        f"--go_out={go_out}", f"--go_opt=module={module}",
        f"--go-grpc_out={go_out}", f"--go-grpc_opt=module={module}",
        *protoc_files,
    ]
    print(" ".join(cmd))
    subprocess.run(cmd, check=True)
