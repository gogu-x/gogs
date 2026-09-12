#!/usr/bin/env python3
# gen-proto.py <proto_dir> <go_out> <module>
#
# 在调用 protoc 之前，自动向每个以 Req 结尾的 message 内联公共头字段。
# 公共头使用 101-103；业务字段保留源 .proto 中声明的字段号。
# 源 .proto 文件保持干净，注入仅发生在服务端生成期。
import re, subprocess, sys, tempfile
from pathlib import Path

proto_dir = Path(sys.argv[1]).resolve()
go_out    = Path(sys.argv[2]).resolve()
module    = sys.argv[3]

# gate 转发前通过 codec.WriteHeader 填充/覆盖这些服务端内部路由字段。
REQ_HEADER_FIELDS = [
    "  uint64   uID       = 101;",
    "  uint32   serverID  = 102;",
    "  string   sessionID = 103;",
]

# 服务间协议不需要通用客户端请求头。
EXCLUDE_MODULES = {"sspb", "pfpb"}


def _transform_req_block(body, name):
    """向 XxxReq 注入高位公共头，不改变任何现有业务字段号。"""
    m = re.match(r"^(\s*message\s+\w+\s*\{)([\s\S]*?)(\}\s*)$", body)
    if not m:
        return body
    head, inner, tail = m.groups()
    if re.search(r"\buID\s*=", inner):
        return body  # 已注入过，跳过
    lines = inner.split("\n")
    out = []
    inserted = False
    for ln in lines:
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
