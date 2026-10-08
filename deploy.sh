#!/bin/sh
set -eu

# 从任何工作目录调用都使用脚本旁的 compose.yaml。
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$ROOT"
command -v docker >/dev/null 2>&1 || { printf '%s\n' '需要安装 Docker Engine。' >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { printf '%s\n' '需要安装 curl。' >&2; exit 1; }
docker compose version >/dev/null 2>&1 || { printf '%s\n' '需要 Docker Compose v2+（支持 --wait）。' >&2; exit 1; }
docker info >/dev/null 2>&1 || { printf '%s\n' '无法连接 Docker，请检查权限或服务状态。' >&2; exit 1; }

# 防止两个更新同时替换同一容器；异常退出也释放锁。
LOCK="$ROOT/.deploy-lock"
mkdir "$LOCK" 2>/dev/null || { printf '%s\n' '已有部署正在运行；若曾被强制终止，请确认没有部署进程后删除 .deploy-lock。' >&2; exit 1; }
trap 'rmdir "$LOCK"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if [ ! -f .env ]; then
  umask 077
  PASSWORD=$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')
  [ "${#PASSWORD}" -eq 48 ] || { printf '%s\n' '生成初始密码失败。' >&2; exit 1; }
  {
    printf 'ADMIN_USERNAME=admin\nADMIN_PASSWORD=%s\n' "$PASSWORD"
    printf 'PORT=8080\nBIND_ADDRESS=0.0.0.0\nCOOKIE_SECURE=false\nPUBLIC_ORIGIN=\n'
  } > .env
  unset PASSWORD
  printf '%s\n' "已生成 $ROOT/.env；初始 admin 密码在该文件中，不会写入部署日志。"
fi
chmod 600 .env

# 先检查配置并完成全部编译/后端测试，构建失败不动正在运行的服务。
docker compose config --quiet
printf '%s\n' '构建前端与 Go 后端（含后端测试）…'
docker compose build --pull
printf '%s\n' '更新服务并等待数据库健康检查…'
if ! docker compose up --detach --remove-orphans --wait --wait-timeout 120; then
  printf '%s\n' '部署未通过健康检查；保留数据卷，请检查以下日志。' >&2
  docker compose ps
  docker compose logs --no-color --tail=100 nicetools
  exit 1
fi

PUBLISHED=$(docker compose port nicetools 8080)
HOST_PORT=${PUBLISHED##*:}
HOST_BIND=${PUBLISHED%:*}
case "$HOST_BIND" in
  0.0.0.0|\[::\]) HOST_BIND=127.0.0.1 ;;
esac
URL="http://$HOST_BIND:$HOST_PORT/api/health"
if ! curl --fail --silent --show-error --max-time 10 "$URL"; then
  printf '\n%s\n' '容器已启动，但宿主机健康请求失败。' >&2
  docker compose logs --no-color --tail=100 nicetools
  exit 1
fi
printf '\n%s\n' "部署成功，健康检查通过；外部端口 $HOST_PORT。"
