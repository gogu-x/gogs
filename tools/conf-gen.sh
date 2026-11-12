#!/usr/bin/env bash
# 从当前包向上寻找 Go 模块根目录，沿用项目配置，只生成代码，不导入数据库。
set -euo pipefail
module_root="$PWD"
while [[ ! -f "$module_root/go.mod" ]]; do
    if [[ "$module_root" == / ]]; then
        echo "conf-gen：当前目录不在 Go 模块内。" >&2
        exit 1
    fi
    module_root="$(dirname -- "$module_root")"
done
exec make -C "$module_root" conf-code "$@"
