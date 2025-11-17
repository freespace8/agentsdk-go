#!/bin/bash
# 运行时测试验证所有示例

set +e  # 不要在错误时立即退出，继续测试

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

echo "=================================="
echo "  运行时测试所有示例"
echo "=================================="
echo ""

# 加载环境变量
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | grep ANTHROPIC | xargs)
    echo "✓ 已加载 .env 配置"
    echo "  API Key: ${ANTHROPIC_API_KEY:0:20}..."
    echo "  Base URL: $ANTHROPIC_BASE_URL"
    echo ""
fi

# 测试结果记录
declare -A TEST_RESULTS
declare -A TEST_OUTPUTS
declare -A TEST_ERRORS

# 测试超时时间（秒）
TIMEOUT=30

# 测试函数
run_test() {
    local example=$1
    local test_type=$2
    local timeout=${3:-$TIMEOUT}

    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "测试: $example ($test_type)"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    local output_file="/tmp/agentsdk_test_${example}_$$"
    local exit_code=0

    case $test_type in
        "api")
            # 需要 API 的示例
            timeout $timeout go run "./examples/$example/main.go" > "$output_file" 2>&1
            exit_code=$?
            ;;
        "http")
            # HTTP 服务示例（启动并测试健康检查）
            go run "./examples/$example/main.go" > "$output_file" 2>&1 &
            local pid=$!
            sleep 3  # 等待服务启动

            # 测试健康检查
            if curl -s http://localhost:8080/health > /dev/null 2>&1; then
                echo "✓ HTTP 服务启动成功" >> "$output_file"
                exit_code=0
            else
                echo "✗ HTTP 服务启动失败" >> "$output_file"
                exit_code=1
            fi

            kill $pid 2>/dev/null
            wait $pid 2>/dev/null
            ;;
        "quick")
            # 快速本地示例（不需要 API）
            timeout $timeout go run "./examples/$example/main.go" > "$output_file" 2>&1
            exit_code=$?
            ;;
        *)
            echo "未知测试类型: $test_type" > "$output_file"
            exit_code=99
            ;;
    esac

    # 读取输出
    local output=$(cat "$output_file" 2>/dev/null | head -50)
    rm -f "$output_file"

    # 分析结果
    if [ $exit_code -eq 0 ]; then
        TEST_RESULTS[$example]="✅ PASS"
        TEST_OUTPUTS[$example]="成功"
    elif [ $exit_code -eq 124 ]; then
        TEST_RESULTS[$example]="⏱️  TIMEOUT"
        TEST_OUTPUTS[$example]="超时（${timeout}s）"
    else
        TEST_RESULTS[$example]="❌ FAIL"
        TEST_ERRORS[$example]="Exit code: $exit_code"
    fi

    # 显示部分输出
    if [ -n "$output" ]; then
        echo "$output" | head -20
    fi

    echo ""
    echo "结果: ${TEST_RESULTS[$example]}"
    echo ""
}

# 定义测试配置
# 格式：示例名:测试类型:超时时间
TEST_CONFIGS=(
    "approval:quick:10"
    "basic:api:30"
    "checkpoint:api:30"
    "custom-tools:api:30"
    "http-full:http:10"
    "http-simple:http:10"
    "http-stream:http:10"
    "mcp:quick:10"
    "model-openai:api:30"
    "security:quick:10"
    "simple-stream:api:30"
    "stream:api:30"
    "telemetry:api:30"
    "tool-basic:api:30"
    "tool-stream:api:30"
    "wal:quick:10"
    "workflow:quick:10"
)

# 运行所有测试
for config in "${TEST_CONFIGS[@]}"; do
    IFS=':' read -r example test_type timeout <<< "$config"
    run_test "$example" "$test_type" "$timeout"
done

# 生成测试报告
echo "================================================"
echo "  测试报告"
echo "================================================"
echo ""

PASSED=0
FAILED=0
TIMEOUT_COUNT=0

for example in "${!TEST_RESULTS[@]}"; do
    result="${TEST_RESULTS[$example]}"
    echo "$result  $example"

    case $result in
        *PASS*)
            PASSED=$((PASSED + 1))
            ;;
        *FAIL*)
            FAILED=$((FAILED + 1))
            ;;
        *TIMEOUT*)
            TIMEOUT_COUNT=$((TIMEOUT_COUNT + 1))
            ;;
    esac
done

echo ""
echo "================================================"
echo "  统计"
echo "================================================"
echo "总数: ${#TEST_CONFIGS[@]}"
echo "通过: $PASSED"
echo "失败: $FAILED"
echo "超时: $TIMEOUT_COUNT"

if [ $FAILED -gt 0 ]; then
    echo ""
    echo "失败详情:"
    for example in "${!TEST_ERRORS[@]}"; do
        echo "  ❌ $example: ${TEST_ERRORS[$example]}"
    done
fi

echo ""
if [ $FAILED -eq 0 ]; then
    echo "✅ 所有测试通过！"
    exit 0
else
    echo "❌ 有 $FAILED 个测试失败"
    exit 1
fi
