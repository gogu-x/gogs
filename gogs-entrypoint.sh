#!/bin/sh
service=
selected=0
remaining=$#
# 已检查的业务参数移到队尾，保持顺序、空值和参数边界。
while [ "$remaining" -gt 0 ]; do
    arg=$1
    shift
    remaining=$((remaining - 1))
    case "$arg" in
        --service-type|--service-type=*)
            if [ "$selected" -eq 1 ]; then echo "Duplicate --service-type" >&2; exit 64; fi
            selected=1
            if [ "$arg" = --service-type ]; then
                if [ "$remaining" -eq 0 ]; then echo "Missing --service-type value" >&2; exit 64; fi
                service=$1
                shift
                remaining=$((remaining - 1))
            else
                service=${arg#--service-type=}
            fi
            ;;
        *) set -- "$@" "$arg" ;;
    esac
done
case "$service" in
    game|battle|gate|platform) ;;
    pf) service=platform ;;
    *) echo "Expected --service-type game, battle, gate or platform (pf)." >&2; exit 64 ;;
esac
exec "/usr/local/bin/$service" "$@"
