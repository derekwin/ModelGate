#!/bin/bash

# ModelGate 自动化运行脚本
# 用法:
#   ./run.sh                      # 默认：只启动 Gateway
#   ./run.sh gateway              # 只启动 Gateway
#   ./run.sh admin                # 启动 Gateway + 管理后台
#   ./run.sh -c configs/dev.yaml  # 使用指定配置文件

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# 默认配置文件
CONFIG_FILE="configs/config.yaml"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 解析命令行参数
parse_args() {
    MODE=""
    while [[ $# -gt 0 ]]; do
        case $1 in
            -c|--config)
                CONFIG_FILE="$2"
                shift 2
                ;;
            gateway|admin|all|stop|restart|status|logs|build|help)
                MODE="$1"
                shift
                ;;
            -h|--help)
                MODE="help"
                shift
                ;;
            *)
                if [ -z "$MODE" ]; then
                    MODE="$1"
                fi
                shift
                ;;
        esac
    done
    
    # 默认模式
    MODE=${MODE:-gateway}
}

# 检查配置文件
check_config() {
    if [ ! -f "$CONFIG_FILE" ]; then
        log_warn "配置文件不存在：$CONFIG_FILE"
        log_info "使用默认配置启动"
    else
        log_info "使用配置文件：$CONFIG_FILE"
    fi
}

# 检查是否已编译
check_binary() {
    if [ ! -f "./modelgate-admin" ]; then
        log_warn "未找到可执行文件，开始编译..."
        go build -o modelgate-admin ./cmd/server/main.go
        if [ $? -eq 0 ]; then
            log_success "编译完成"
        else
            log_error "编译失败"
            exit 1
        fi
    fi
}

# 停止已有进程
stop_existing() {
    log_info "检查并停止已有进程..."
    pkill -f modelgate-admin 2>/dev/null || true
    sleep 1
}

# 启动 Gateway
start_gateway() {
    log_info "启动 ModelGate Gateway..."
    log_info "配置文件：$CONFIG_FILE"
    
    nohup ./modelgate-admin -c "$CONFIG_FILE" > logs/gateway.log 2>&1 &
    GATEWAY_PID=$!
    echo $GATEWAY_PID > logs/gateway.pid
    sleep 2
    
    if ps -p $GATEWAY_PID > /dev/null 2>&1; then
        log_success "Gateway 已启动 (PID: $GATEWAY_PID)"
        log_info "日志文件：logs/gateway.log"
    else
        log_error "Gateway 启动失败，请查看日志"
        exit 1
    fi
}

# 显示帮助
show_help() {
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  gateway         只启动 Gateway (默认)"
    echo "  admin           启动 Gateway + 管理后台"
    echo "  all             启动 Gateway + 管理后台 (同 admin)"
    echo "  stop            停止所有服务"
    echo "  restart         重启所有服务"
    echo "  status          查看服务状态"
    echo "  logs            查看日志"
    echo "  build           重新编译"
    echo "  help            显示帮助信息"
    echo ""
    echo "配置文件选项:"
    echo "  -c, --config    指定配置文件路径 (默认：configs/config.yaml)"
    echo ""
    echo "示例:"
    echo "  $0                                    # 启动 Gateway (默认配置)"
    echo "  $0 admin                              # 启动 Gateway + 管理后台"
    echo "  $0 -c configs/dev.yaml                # 使用开发配置启动"
    echo "  $0 admin -c configs/production.yaml   # 使用生产配置启动"
    echo "  $0 stop                               # 停止所有服务"
    echo ""
    echo "访问地址:"
    echo "  Gateway API:    http://ip:8080/v1/"
    echo "  管理后台：      http://ip:8080/admin/"
}

# 查看状态
show_status() {
    log_info "服务状态:"
    echo ""
    
    if [ -f "logs/gateway.pid" ]; then
        GATEWAY_PID=$(cat logs/gateway.pid)
        if ps -p $GATEWAY_PID > /dev/null 2>&1; then
            echo -e "  Gateway: ${GREEN}运行中${NC} (PID: $GATEWAY_PID)"
        else
            echo -e "  Gateway: ${RED}已停止${NC}"
        fi
    else
        echo -e "  Gateway: ${YELLOW}未找到 PID 文件${NC}"
    fi
    
    echo ""
    echo "端口占用:"
    if command -v lsof > /dev/null 2>&1; then
        lsof -i :8080 2>/dev/null | grep LISTEN || echo "  8080 端口未被占用"
    else
        netstat -tuln 2>/dev/null | grep :8080 || echo "  8080 端口未被占用"
    fi
}

# 查看日志
show_logs() {
    if [ ! -d "logs" ]; then
        log_error "日志目录不存在"
        exit 1
    fi
    
    echo "日志文件:"
    ls -lh logs/*.log 2>/dev/null || echo "  无日志文件"
    echo ""
    
    if [ -n "$1" ]; then
        case $1 in
            gateway)
                tail -f logs/gateway.log 2>/dev/null || log_error "Gateway 日志不存在"
                ;;
            *)
                tail -f logs/$1.log 2>/dev/null || log_error "日志文件不存在：$1"
                ;;
        esac
    else
        log_info "默认显示 Gateway 日志 (Ctrl+C 退出)"
        tail -f logs/gateway.log 2>/dev/null
    fi
}

# 停止服务
stop_all() {
    log_info "停止所有服务..."
    
    if [ -f "logs/gateway.pid" ]; then
        GATEWAY_PID=$(cat logs/gateway.pid)
        if ps -p $GATEWAY_PID > /dev/null 2>&1; then
            kill $GATEWAY_PID 2>/dev/null || true
            log_success "Gateway 已停止 (PID: $GATEWAY_PID)"
        fi
        rm -f logs/gateway.pid
    fi
    
    pkill -f modelgate-admin 2>/dev/null || true
    log_success "所有服务已停止"
}

# 重启服务
restart_all() {
    log_info "重启所有服务..."
    stop_all
    sleep 2
    start_gateway
}

# 重新编译
build_binary() {
    log_info "重新编译..."
    go build -o modelgate-admin ./cmd/server/main.go
    if [ $? -eq 0 ]; then
        log_success "编译完成"
    else
        log_error "编译失败"
        exit 1
    fi
}

# 主程序
main() {
    # 解析参数
    parse_args "$@"
    
    # 创建日志目录
    mkdir -p logs
    
    case $MODE in
        gateway)
            log_info "启动模式：Gateway Only"
            check_binary
            check_config
            stop_existing
            start_gateway
            log_success "启动完成!"
            echo ""
            echo "访问地址:"
            echo "  Gateway API: http://ip:8080/v1/"
            echo ""
            echo "配置文件：$CONFIG_FILE"
            echo "使用 '$0 stop' 停止服务"
            ;;
        
        admin|all)
            log_info "启动模式：Gateway + Admin"
            check_binary
            check_config
            stop_existing
            start_gateway
            log_success "启动完成!"
            echo ""
            echo "访问地址:"
            echo "  Gateway API:    http://ip:8080/v1/"
            echo "  管理后台：      http://ip:8080/admin/"
            echo ""
            echo "配置文件：$CONFIG_FILE"
            echo "使用 '$0 stop' 停止服务"
            ;;
        
        stop)
            stop_all
            ;;
        
        restart)
            restart_all
            ;;
        
        status)
            show_status
            ;;
        
        logs)
            show_logs "$2"
            ;;
        
        build)
            build_binary
            ;;
        
        help|-h|--help)
            show_help
            ;;
        
        *)
            log_error "未知选项：$MODE"
            echo ""
            show_help
            exit 1
            ;;
    esac
}

# 执行主程序
main "$@"
