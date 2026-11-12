#!/usr/bin/env bash
# 构建并安装当前仓库的代码生成器；默认安装到用户 PATH 目录。
set -euo pipefail
repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
generator_bin_dir="${1:-${HOME}/.local/bin}"
mkdir -p -- "$generator_bin_dir"
cd -- "$repo_root"
modules=(accessor_gen deepcopy-gen router-gen)
binaries=(accessor-gen deepcopy-gen router-gen)
commands=(accessor-gen deepcopy-gen actv-router-gen)
# 配置生成入口沿用项目 Makefile，仅自动定位模块根目录。
conf_target="$generator_bin_dir/conf-gen"
if [[ -e "$conf_target" || -L "$conf_target" ]]; then
    if [[ ! -L "$conf_target" || "$(readlink -- "$conf_target")" != "$repo_root/tools/conf-gen.sh" ]]; then
        echo "拒绝覆盖已有工具：$conf_target" >&2
        exit 1
    fi
fi
# 先检查目标，避免覆盖其他项目已经安装的工具。
for i in "${!commands[@]}"; do
    target="$generator_bin_dir/${commands[$i]}"
    source_binary="$repo_root/tools/${modules[$i]}/${binaries[$i]}"
    if [[ -e "$target" || -L "$target" ]]; then
        if [[ ! -L "$target" || "$(readlink -- "$target")" != "$source_binary" ]]; then
            echo "拒绝覆盖已有工具：$target" >&2
            exit 1
        fi
    fi
done
for i in "${!commands[@]}"; do
    source_binary="$repo_root/tools/${modules[$i]}/${binaries[$i]}"
    go build -o "$source_binary" "./tools/${modules[$i]}"
    ln -sfn -- "$source_binary" "$generator_bin_dir/${commands[$i]}"
    echo "已安装 ${commands[$i]} -> $source_binary"
done
ln -sfn -- "$repo_root/tools/conf-gen.sh" "$conf_target"
echo "已安装 conf-gen -> $repo_root/tools/conf-gen.sh"
case ":$PATH:" in
    *":$generator_bin_dir:"*) ;;
    *) echo "请将 $generator_bin_dir 加入 PATH。" ;;
esac
