#!/bin/bash
# 编译验证所有示例

set -e  # 遇到错误立即退出

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "=================================="
echo "  编译验证所有示例"
echo "=================================="
echo ""

# 定义所有示例
EXAMPLES=(
    "approval"
    "basic"
    "checkpoint"
    "custom-tools"
    "http-full"
    "http-simple"
    "http-stream"
    "mcp"
    "model-openai"
    "security"
    "simple-stream"
    "stream"
    "telemetry"
    "tool-basic"
    "tool-stream"
    "wal"
    "workflow"
)

SUCCESS=0
FAILED=0
FAILED_EXAMPLES=()

for example in "${EXAMPLES[@]}"; do
    echo -n "[$example] 编译中... "

    if go build -o /dev/null "./examples/$example/main.go" 2>&1 | grep -q "error"; then
        echo "❌ 失败"
        FAILED=$((FAILED + 1))
        FAILED_EXAMPLES+=("$example")
    else
        echo "✅ 成功"
        SUCCESS=$((SUCCESS + 1))
    fi
done

echo ""
echo "=================================="
echo "  编译结果统计"
echo "=================================="
echo "总数: ${#EXAMPLES[@]}"
echo "成功: $SUCCESS"
echo "失败: $FAILED"

if [ $FAILED -gt 0 ]; then
    echo ""
    echo "失败的示例:"
    for example in "${FAILED_EXAMPLES[@]}"; do
        echo "  - $example"
    done
    exit 1
fi

echo ""
echo "✅ 所有示例编译成功！"
